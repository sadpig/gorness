package tools

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"gorness.dev/gorness/internal/store"
)

var (
	ErrStalePrecondition = errors.New("stale file precondition: file modified externally since last read")
	ErrOldStringNotFound = errors.New("old_string not found in target file")
	ErrAmbiguousMatch    = errors.New("old_string matched multiple locations; add surrounding context or set replace_all=true")
	ErrNoOpEdit          = errors.New("edit produces no content changes")
)

const (
	hunkCoalesceMaxGap = 2
	hunkContextLines   = 2
	myersMaxMatrixOps  = 4096
)

type DiffHunk struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
	Header      string `json:"header"`
	OldStart    int    `json:"old_start"`
	OldLines    int    `json:"old_lines"`
	NewStart    int    `json:"new_start"`
	NewLines    int    `json:"new_lines"`
	Preview     string `json:"preview"`
}

type hunkSpan struct {
	id       string
	oldStart int
	oldEnd   int
	newStart int
	newEnd   int
}

type StagedEdit struct {
	AbsPath          string           `json:"abs_path"`
	RelPath          string           `json:"rel_path"`
	Kind             store.ChangeKind `json:"kind"`
	BeforeBytes      []byte           `json:"-"`
	ProposedBytes    []byte           `json:"-"`
	PreconditionHash string           `json:"precondition_hash"`
	Hunks            []DiffHunk       `json:"hunks"`

	canonAbs string
	spans    []hunkSpan
}

// ComputeStagedEdit 执行 SHA-256 乐观并发前置校验与 4 级容错匹配级联：
// 1. 精确字节/子串匹配 -> 2. 行尾空白归一化匹配 -> 3. 弯引号/破折号/特殊空格归一化匹配 -> 4. 相对缩进保持匹配。
func ComputeStagedEdit(absPath, relPath, oldStr, newStr string, replaceAll bool, readHash string) (*StagedEdit, error) {
	if strings.TrimSpace(absPath) == "" {
		return nil, fmt.Errorf("%w: empty file path", ErrInvalidTargetPath)
	}
	if oldStr == newStr {
		return nil, ErrNoOpEdit
	}

	st, statErr := os.Lstat(absPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("lstat %s: %w", absPath, statErr)
	}
	if statErr == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: refusing to edit symlink %s", ErrInvalidTargetPath, relPath)
		}
		if st.IsDir() {
			return nil, fmt.Errorf("%w: target %s is a directory", ErrInvalidTargetPath, relPath)
		}
	}

	expectedHash := strings.ToLower(strings.TrimSpace(readHash))

	// 文件尚不存在且 oldStr 为空时视为新建文件。
	if errors.Is(statErr, os.ErrNotExist) {
		if oldStr != "" {
			return nil, fmt.Errorf("cannot edit non-existent file %s with non-empty old_string: %w", relPath, os.ErrNotExist)
		}
		if expectedHash != "" {
			return nil, fmt.Errorf("%w: expected %s for %s, but file does not exist", ErrStalePrecondition, expectedHash, relPath)
		}
		return buildStagedResult(absPath, relPath, store.ChangeCreate, nil, []byte(newStr), ""), nil
	}

	beforeBytes, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}
	actualHash := sha256HexBytes(beforeBytes)
	if expectedHash != "" && actualHash != expectedHash {
		return nil, fmt.Errorf("%w: %s expected %s, got %s", ErrStalePrecondition, relPath, expectedHash, actualHash)
	}

	if len(beforeBytes) == 0 && oldStr == "" {
		return buildStagedResult(absPath, relPath, store.ChangeModify, beforeBytes, []byte(newStr), actualHash), nil
	}
	if oldStr == "" {
		return nil, ErrOldStringNotFound
	}

	rawText := string(beforeBytes)

	// 若仅修改换行符（\r\n <-> \n），直接对原始字节执行精确替换。
	if strings.ReplaceAll(oldStr, "\r\n", "\n") == strings.ReplaceAll(newStr, "\r\n", "\n") {
		replaced, ok, mErr := matchStage1Exact(rawText, oldStr, newStr, replaceAll)
		if mErr != nil {
			return nil, mErr
		}
		if !ok {
			return nil, ErrOldStringNotFound
		}
		return buildStagedResult(absPath, relPath, store.ChangeModify, beforeBytes, []byte(replaced), actualHash), nil
	}

	useCRLF := hasDominantCRLF(rawText)
	docLF := strings.ReplaceAll(rawText, "\r\n", "\n")
	oldLF := strings.ReplaceAll(oldStr, "\r\n", "\n")
	newLF := strings.ReplaceAll(newStr, "\r\n", "\n")

	proposedLF, err := applyFourStageCascade(docLF, oldLF, newLF, replaceAll)
	if err != nil {
		return nil, err
	}
	if proposedLF == docLF {
		return nil, ErrNoOpEdit
	}

	proposedFinal := proposedLF
	if useCRLF {
		proposedFinal = strings.ReplaceAll(proposedLF, "\n", "\r\n")
	}
	return buildStagedResult(absPath, relPath, store.ChangeModify, beforeBytes, []byte(proposedFinal), actualHash), nil
}

// ComputeStagedWrite 为 write_file 生成带统一 DiffHunk 与前置哈希保护的暂存变更。
func ComputeStagedWrite(absPath, relPath string, content []byte, readHash string) (*StagedEdit, error) {
	if strings.TrimSpace(absPath) == "" {
		return nil, fmt.Errorf("%w: empty file path", ErrInvalidTargetPath)
	}
	st, statErr := os.Lstat(absPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("lstat %s: %w", absPath, statErr)
	}
	expectedHash := strings.ToLower(strings.TrimSpace(readHash))

	if errors.Is(statErr, os.ErrNotExist) {
		if expectedHash != "" {
			return nil, fmt.Errorf("%w: expected %s for %s, but file does not exist", ErrStalePrecondition, expectedHash, relPath)
		}
		return buildStagedResult(absPath, relPath, store.ChangeCreate, nil, append([]byte(nil), content...), ""), nil
	}

	if st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: refusing to overwrite symlink %s", ErrInvalidTargetPath, relPath)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%w: refusing to overwrite directory %s", ErrInvalidTargetPath, relPath)
	}

	beforeBytes, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}
	actualHash := sha256HexBytes(beforeBytes)
	if expectedHash != "" && actualHash != expectedHash {
		return nil, fmt.Errorf("%w: %s expected %s, got %s", ErrStalePrecondition, relPath, expectedHash, actualHash)
	}

	return buildStagedResult(absPath, relPath, store.ChangeModify, beforeBytes, append([]byte(nil), content...), actualHash), nil
}

// ApplySelectedHunks 支持全选或部分勾选 DiffHunk 合并落盘：
// - 若 selectedHunkIDs 为空切片或未命中任何块，绝不触碰磁盘并返回 written=false；
// - 写入前再次校验磁盘文件 SHA-256 与 PreconditionHash 一致，防止外部编辑器改动被静默覆盖；
// - 落盘前自动写入 SnapshotStore 内容寻址块与会话步变更账本，支撑 /rewind 毫秒级还原。
func ApplySelectedHunks(
	staged *StagedEdit,
	selectedHunkIDs []string,
	snaps *store.SnapshotStore,
	sessionID string,
	seq uint64,
) (written bool, applied int, err error) {
	if staged == nil {
		return false, 0, errors.New("nil staged edit")
	}
	if len(selectedHunkIDs) == 0 {
		return false, 0, nil
	}

	wantSet := make(map[string]bool, len(selectedHunkIDs))
	for _, id := range selectedHunkIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			wantSet[trimmed] = true
		}
	}
	if len(wantSet) == 0 {
		return false, 0, nil
	}

	spans := staged.spans
	if len(spans) == 0 && len(staged.Hunks) > 0 {
		_, spans = computeHunksAndSpans(staged.RelPath, staged.Kind, staged.BeforeBytes, staged.ProposedBytes)
	}

	matched := 0
	for _, sp := range spans {
		if wantSet[sp.id] {
			matched++
		}
	}
	if matched == 0 {
		return false, 0, nil
	}

	// 写入前执行原子前置哈希与规范路径二次校验，严防审批窗口期内发生外部修改或父目录软链掉包。
	if staged.canonAbs != "" {
		nowCanon, cErr := resolveCanonicalPath(staged.AbsPath, 0)
		if cErr != nil {
			return false, 0, cErr
		}
		if nowCanon != staged.canonAbs {
			return false, 0, fmt.Errorf("%w: canonical target changed before apply (%s -> %s)", ErrStalePrecondition, staged.canonAbs, nowCanon)
		}
	}

	perm := os.FileMode(0o644)
	st, statErr := os.Lstat(staged.AbsPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return false, 0, fmt.Errorf("lstat %s: %w", staged.AbsPath, statErr)
	}
	if statErr == nil && st.Mode()&os.ModeSymlink != 0 {
		return false, 0, fmt.Errorf("%w: refusing to write symlink %s", ErrInvalidTargetPath, staged.RelPath)
	}

	switch staged.Kind {
	case store.ChangeCreate:
		if statErr == nil {
			return false, 0, fmt.Errorf("%w: %s was created externally before apply", ErrStalePrecondition, staged.RelPath)
		}
	case store.ChangeModify, store.ChangeDelete:
		if errors.Is(statErr, os.ErrNotExist) {
			return false, 0, fmt.Errorf("%w: %s was removed externally before apply", ErrStalePrecondition, staged.RelPath)
		}
		perm = st.Mode().Perm()
		diskBytes, rErr := os.ReadFile(staged.AbsPath)
		if rErr != nil {
			return false, 0, fmt.Errorf("read %s for precondition check: %w", staged.AbsPath, rErr)
		}
		diskHash := sha256HexBytes(diskBytes)
		expected := strings.ToLower(strings.TrimSpace(staged.PreconditionHash))
		if expected == "" {
			expected = sha256HexBytes(staged.BeforeBytes)
		}
		if diskHash != expected {
			return false, 0, fmt.Errorf("%w: %s changed externally (expected %s, got %s)", ErrStalePrecondition, staged.RelPath, expected, diskHash)
		}
	default:
		return false, 0, fmt.Errorf("unsupported staged edit kind %q", staged.Kind)
	}

	if staged.Kind == store.ChangeDelete {
		var beforeHash string
		if snaps != nil {
			beforeHash, err = snaps.PutBlob(staged.BeforeBytes)
			if err != nil {
				return false, 0, err
			}
		}
		if err := os.Remove(staged.AbsPath); err != nil {
			return false, 0, fmt.Errorf("remove %s: %w", staged.AbsPath, err)
		}
		if snaps != nil && strings.TrimSpace(sessionID) != "" && strings.TrimSpace(staged.RelPath) != "" && staged.RelPath != "." {
			if err := snaps.RecordStepChange(store.FileSnapshotRecord{
				Seq:        seq,
				SessionID:  sessionID,
				RelPath:    staged.RelPath,
				Kind:       store.ChangeDelete,
				BeforeHash: beforeHash,
			}); err != nil {
				return false, 0, err
			}
		}
		return true, matched, nil
	}

	finalBytes := mergeSelectedHunkTokens(staged.BeforeBytes, staged.ProposedBytes, spans, wantSet)
	if staged.Kind == store.ChangeModify && bytes.Equal(finalBytes, staged.BeforeBytes) {
		return false, 0, nil
	}

	var beforeHash, afterHash string
	if snaps != nil {
		if staged.Kind == store.ChangeModify {
			beforeHash, err = snaps.PutBlob(staged.BeforeBytes)
			if err != nil {
				return false, 0, err
			}
		}
		afterHash, err = snaps.PutBlob(finalBytes)
		if err != nil {
			return false, 0, err
		}
	}

	if err := store.AtomicWriteFile(staged.AbsPath, finalBytes, perm); err != nil {
		return false, 0, err
	}

	if snaps != nil && strings.TrimSpace(sessionID) != "" && strings.TrimSpace(staged.RelPath) != "" && staged.RelPath != "." {
		if err := snaps.RecordStepChange(store.FileSnapshotRecord{
			Seq:        seq,
			SessionID:  sessionID,
			RelPath:    staged.RelPath,
			Kind:       staged.Kind,
			BeforeHash: beforeHash,
			AfterHash:  afterHash,
		}); err != nil {
			return false, 0, err
		}
	}

	return true, matched, nil
}

func buildStagedResult(
	absPath, relPath string,
	kind store.ChangeKind,
	beforeBytes, proposedBytes []byte,
	preconditionHash string,
) *StagedEdit {
	hunks, spans := computeHunksAndSpans(relPath, kind, beforeBytes, proposedBytes)
	canonAbs, _ := resolveCanonicalPath(absPath, 0)
	return &StagedEdit{
		AbsPath:          absPath,
		RelPath:          relPath,
		Kind:             kind,
		BeforeBytes:      append([]byte(nil), beforeBytes...),
		ProposedBytes:    append([]byte(nil), proposedBytes...),
		PreconditionHash: preconditionHash,
		Hunks:            hunks,
		canonAbs:         canonAbs,
		spans:            spans,
	}
}

func applyFourStageCascade(doc, oldStr, newStr string, replaceAll bool) (string, error) {
	// Stage 1: Exact match
	if out, ok, err := matchStage1Exact(doc, oldStr, newStr, replaceAll); err != nil {
		return "", err
	} else if ok {
		return out, nil
	}

	// Stage 2: Trailing-space normalized line match
	if out, ok, err := matchStage2TrailingSpace(doc, oldStr, newStr, replaceAll); err != nil {
		return "", err
	} else if ok {
		return out, nil
	}

	// Stage 3: Typographic quote / dash / space normalized match
	if out, ok, err := matchStage3Typographic(doc, oldStr, newStr, replaceAll); err != nil {
		return "", err
	} else if ok {
		return out, nil
	}

	// Stage 4: Relative indentation match
	if out, ok, err := matchStage4RelativeIndent(doc, oldStr, newStr, replaceAll); err != nil {
		return "", err
	} else if ok {
		return out, nil
	}

	return "", ErrOldStringNotFound
}

func matchStage1Exact(doc, oldStr, newStr string, replaceAll bool) (string, bool, error) {
	count := strings.Count(doc, oldStr)
	if count == 0 {
		return "", false, nil
	}
	if count > 1 && !replaceAll {
		return "", false, ErrAmbiguousMatch
	}
	if replaceAll {
		return strings.ReplaceAll(doc, oldStr, newStr), true, nil
	}
	return strings.Replace(doc, oldStr, newStr, 1), true, nil
}

func matchStage2TrailingSpace(doc, oldStr, newStr string, replaceAll bool) (string, bool, error) {
	return matchLineWindows(doc, oldStr, newStr, replaceAll, func(line string) string {
		return strings.TrimRight(line, " \t")
	})
}

func matchStage3Typographic(doc, oldStr, newStr string, replaceAll bool) (string, bool, error) {
	// 优先支持行内子串级弯引号/破折号归一化匹配（例如单行内部分短语替换）。
	if !strings.Contains(oldStr, "\n") {
		if out, ok, err := matchInlineTypographic(doc, oldStr, newStr, replaceAll); err != nil {
			return "", false, err
		} else if ok {
			return out, true, nil
		}
	}
	return matchLineWindows(doc, oldStr, newStr, replaceAll, func(line string) string {
		return normalizeTypographic(strings.TrimRight(line, " \t"))
	})
}

func matchInlineTypographic(doc, oldStr, newStr string, replaceAll bool) (string, bool, error) {
	normOld := normalizeTypographic(oldStr)
	if normOld == "" {
		return "", false, nil
	}

	var normDoc strings.Builder
	normToOrigStart := make([]int, 0, len(doc)+1)
	for byteIdx, r := range doc {
		repl := normalizeTypographicRune(r)
		for i := 0; i < len(repl); i++ {
			normToOrigStart = append(normToOrigStart, byteIdx)
		}
		normDoc.WriteString(repl)
	}
	normToOrigStart = append(normToOrigStart, len(doc))
	normStr := normDoc.String()

	type byteRange struct{ start, end int }
	var matches []byteRange
	searchFrom := 0
	for searchFrom <= len(normStr)-len(normOld) {
		idx := strings.Index(normStr[searchFrom:], normOld)
		if idx < 0 {
			break
		}
		nStart := searchFrom + idx
		nEnd := nStart + len(normOld)
		origStart := normToOrigStart[nStart]
		origEnd := normToOrigStart[nEnd]
		if origEnd > origStart {
			matches = append(matches, byteRange{start: origStart, end: origEnd})
		}
		searchFrom = nEnd
	}

	if len(matches) == 0 {
		return "", false, nil
	}
	if len(matches) > 1 && !replaceAll {
		return "", false, ErrAmbiguousMatch
	}

	var out strings.Builder
	cursor := 0
	for i, m := range matches {
		if i > 0 && !replaceAll {
			break
		}
		out.WriteString(doc[cursor:m.start])
		out.WriteString(newStr)
		cursor = m.end
	}
	out.WriteString(doc[cursor:])
	return out.String(), true, nil
}

func matchLineWindows(
	doc, oldStr, newStr string,
	replaceAll bool,
	normFn func(string) string,
) (string, bool, error) {
	hasTrailingNL := strings.HasSuffix(doc, "\n")
	docBody := strings.TrimSuffix(doc, "\n")
	patBody := strings.TrimSuffix(oldStr, "\n")
	newBody := strings.TrimSuffix(newStr, "\n")

	docLines := strings.Split(docBody, "\n")
	patLines := strings.Split(patBody, "\n")
	newLines := strings.Split(newBody, "\n")

	if len(patLines) == 0 || len(patLines) > len(docLines) {
		return "", false, nil
	}

	normPat := make([]string, len(patLines))
	for i, l := range patLines {
		normPat[i] = normFn(l)
	}

	var matchStarts []int
	for i := 0; i <= len(docLines)-len(patLines); {
		ok := true
		for j := 0; j < len(patLines); j++ {
			if normFn(docLines[i+j]) != normPat[j] {
				ok = false
				break
			}
		}
		if ok {
			matchStarts = append(matchStarts, i)
			i += len(patLines)
		} else {
			i++
		}
	}

	if len(matchStarts) == 0 {
		return "", false, nil
	}
	if len(matchStarts) > 1 && !replaceAll {
		return "", false, ErrAmbiguousMatch
	}

	var resultLines []string
	cursor := 0
	for idx, start := range matchStarts {
		if idx > 0 && !replaceAll {
			break
		}
		resultLines = append(resultLines, docLines[cursor:start]...)
		// 若首行在原文件中带有前导缩进，而 oldStr 未携带缩进，保持行级替换原样写入 newLines。
		resultLines = append(resultLines, newLines...)
		cursor = start + len(patLines)
	}
	resultLines = append(resultLines, docLines[cursor:]...)

	joined := strings.Join(resultLines, "\n")
	if hasTrailingNL {
		joined += "\n"
	}
	return joined, true, nil
}

func matchStage4RelativeIndent(doc, oldStr, newStr string, replaceAll bool) (string, bool, error) {
	hasTrailingNL := strings.HasSuffix(doc, "\n")
	docBody := strings.TrimSuffix(doc, "\n")
	patBody := strings.TrimSuffix(oldStr, "\n")
	newBody := strings.TrimSuffix(newStr, "\n")

	docLines := strings.Split(docBody, "\n")
	patLines := strings.Split(patBody, "\n")
	newLines := strings.Split(newBody, "\n")

	if len(patLines) == 0 || len(patLines) > len(docLines) {
		return "", false, nil
	}

	patMinCols := minIndentCols(patLines)
	if patMinCols < 0 {
		return "", false, nil
	}

	type indentMatch struct {
		startLine   int
		docBasePref string
	}
	var matches []indentMatch

	for i := 0; i <= len(docLines)-len(patLines); {
		window := docLines[i : i+len(patLines)]
		docMinCols := minIndentCols(window)
		if docMinCols < 0 {
			i++
			continue
		}

		ok := true
		var basePref string
		haveBasePref := false
		for j := 0; j < len(patLines); j++ {
			pTrim := strings.TrimSpace(patLines[j])
			dTrim := strings.TrimSpace(window[j])
			if pTrim == "" && dTrim == "" {
				continue
			}
			if pTrim == "" || dTrim == "" {
				ok = false
				break
			}
			pRelCols := leadingIndentCols(patLines[j]) - patMinCols
			dRelCols := leadingIndentCols(window[j]) - docMinCols
			if pRelCols != dRelCols {
				ok = false
				break
			}
			pStripped := normalizeTypographic(strings.TrimRight(stripCols(patLines[j], leadingIndentCols(patLines[j])), " \t"))
			dStripped := normalizeTypographic(strings.TrimRight(stripCols(window[j], leadingIndentCols(window[j])), " \t"))
			if pStripped != dStripped {
				ok = false
				break
			}
			if !haveBasePref && pRelCols == 0 {
				basePref = leadingWhitespacePrefix(window[j])
				haveBasePref = true
			}
		}

		if ok {
			if !haveBasePref {
				basePref = strings.Repeat(" ", docMinCols)
			}
			matches = append(matches, indentMatch{startLine: i, docBasePref: basePref})
			i += len(patLines)
		} else {
			i++
		}
	}

	if len(matches) == 0 {
		return "", false, nil
	}
	if len(matches) > 1 && !replaceAll {
		return "", false, ErrAmbiguousMatch
	}

	var resultLines []string
	cursor := 0
	for idx, m := range matches {
		if idx > 0 && !replaceAll {
			break
		}
		resultLines = append(resultLines, docLines[cursor:m.startLine]...)
		for _, nl := range newLines {
			if strings.TrimSpace(nl) == "" {
				resultLines = append(resultLines, "")
				continue
			}
			nlCols := leadingIndentCols(nl)
			relCols := nlCols - patMinCols
			if relCols < 0 {
				relCols = 0
			}
			rest := stripCols(nl, nlCols)
			extraIndent := buildRelativeIndent(nl, patMinCols, relCols, m.docBasePref)
			resultLines = append(resultLines, m.docBasePref+extraIndent+rest)
		}
		cursor = m.startLine + len(patLines)
	}
	resultLines = append(resultLines, docLines[cursor:]...)

	joined := strings.Join(resultLines, "\n")
	if hasTrailingNL {
		joined += "\n"
	}
	return joined, true, nil
}

func buildRelativeIndent(line string, baseCols, relCols int, docBasePref string) string {
	if relCols <= 0 {
		return ""
	}
	afterBase := stripCols(line, baseCols)
	relWS := leadingWhitespacePrefix(afterBase)
	if strings.Contains(docBasePref, "\t") && !strings.Contains(relWS, "\t") && relCols%4 == 0 {
		return strings.Repeat("\t", relCols/4)
	}
	if relWS != "" {
		return relWS
	}
	return strings.Repeat(" ", relCols)
}

func leadingWhitespacePrefix(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

func leadingIndentCols(s string) int {
	cols := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			cols++
		case '\t':
			cols += 4
		default:
			return cols
		}
	}
	return cols
}

func stripCols(s string, cols int) string {
	removed := 0
	i := 0
	for i < len(s) && removed < cols {
		switch s[i] {
		case ' ':
			removed++
			i++
		case '\t':
			removed += 4
			i++
		default:
			return s[i:]
		}
	}
	return s[i:]
}

func minIndentCols(lines []string) int {
	minCols := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		c := leadingIndentCols(l)
		if minCols < 0 || c < minCols {
			minCols = c
		}
	}
	return minCols
}

func normalizeTypographic(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteString(normalizeTypographicRune(r))
	}
	return b.String()
}

func normalizeTypographicRune(r rune) string {
	switch r {
	case '\u2018', '\u2019', '\u201A', '\u201B', '\u2032':
		return "'"
	case '\u201C', '\u201D', '\u201E', '\u201F', '\u00AB', '\u00BB', '\u2033':
		return "\""
	case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
		return "-"
	case '\u00A0', '\u2007', '\u202F', '\u3000':
		return " "
	case '\u2026':
		return "..."
	default:
		return string(r)
	}
}

func hasDominantCRLF(s string) bool {
	crlf := strings.Count(s, "\r\n")
	if crlf == 0 {
		return false
	}
	totalLF := strings.Count(s, "\n")
	return crlf*2 >= totalLF
}

// computeHunksAndSpans 基于 Myers O(ND) 行级差分将修改拆分为互不重叠的独立编辑块。
func computeHunksAndSpans(relPath string, kind store.ChangeKind, beforeBytes, proposedBytes []byte) ([]DiffHunk, []hunkSpan) {
	oldTokens := splitLinesKeepEnding(beforeBytes)
	newTokens := splitLinesKeepEnding(proposedBytes)

	if kind == store.ChangeCreate && len(oldTokens) == 0 && len(newTokens) == 0 {
		fp := sha256HexBytes([]byte("create-empty:" + relPath))[:16]
		header := "@@ -0,0 +1,0 @@"
		return []DiffHunk{{
			ID:          "hunk-1",
			Fingerprint: fp,
			Header:      header,
			OldStart:    0,
			OldLines:    0,
			NewStart:    1,
			NewLines:    0,
			Preview:     header,
		}}, []hunkSpan{{id: "hunk-1", oldStart: 0, oldEnd: 0, newStart: 0, newEnd: 0}}
	}

	rawChanges := diffTokenRanges(oldTokens, newTokens)
	if len(rawChanges) == 0 {
		return nil, nil
	}

	// 合并间距不超过 hunkCoalesceMaxGap 行的相邻改动。
	var merged []changeRange
	for _, ch := range rawChanges {
		if len(merged) == 0 {
			merged = append(merged, ch)
			continue
		}
		last := &merged[len(merged)-1]
		gap := ch.oldStart - last.oldEnd
		if gap <= hunkCoalesceMaxGap {
			last.oldEnd = ch.oldEnd
			last.newEnd = ch.newEnd
		} else {
			merged = append(merged, ch)
		}
	}

	hunks := make([]DiffHunk, 0, len(merged))
	spans := make([]hunkSpan, 0, len(merged))

	for idx, m := range merged {
		id := fmt.Sprintf("hunk-%d", idx+1)

		ctxBefore := hunkContextLines
		if m.oldStart < ctxBefore {
			ctxBefore = m.oldStart
		}
		if m.newStart < ctxBefore {
			ctxBefore = m.newStart
		}

		ctxAfter := hunkContextLines
		if remOld := len(oldTokens) - m.oldEnd; remOld < ctxAfter {
			ctxAfter = remOld
		}
		if remNew := len(newTokens) - m.newEnd; remNew < ctxAfter {
			ctxAfter = remNew
		}

		dispOldStartIdx := m.oldStart - ctxBefore
		dispOldEndIdx := m.oldEnd + ctxAfter
		dispNewStartIdx := m.newStart - ctxBefore
		dispNewEndIdx := m.newEnd + ctxAfter

		oldCount := dispOldEndIdx - dispOldStartIdx
		newCount := dispNewEndIdx - dispNewStartIdx

		oldLine1 := dispOldStartIdx + 1
		if len(oldTokens) == 0 && oldCount == 0 {
			oldLine1 = 0
		}
		newLine1 := dispNewStartIdx + 1
		if len(newTokens) == 0 && newCount == 0 {
			newLine1 = 0
		}

		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldLine1, oldCount, newLine1, newCount)

		var preview strings.Builder
		preview.WriteString(header)
		for i := dispOldStartIdx; i < m.oldStart; i++ {
			preview.WriteString("\n ")
			preview.WriteString(trimLineEnding(oldTokens[i]))
		}
		for i := m.oldStart; i < m.oldEnd; i++ {
			preview.WriteString("\n-")
			preview.WriteString(trimLineEnding(oldTokens[i]))
		}
		for i := m.newStart; i < m.newEnd; i++ {
			preview.WriteString("\n+")
			preview.WriteString(trimLineEnding(newTokens[i]))
		}
		for i := m.oldEnd; i < dispOldEndIdx; i++ {
			preview.WriteString("\n ")
			preview.WriteString(trimLineEnding(oldTokens[i]))
		}

		var fpSrc bytes.Buffer
		fpSrc.WriteString(relPath)
		fpSrc.WriteByte(0)
		for i := m.oldStart; i < m.oldEnd; i++ {
			fpSrc.WriteString(oldTokens[i])
		}
		fpSrc.WriteByte(0)
		for i := m.newStart; i < m.newEnd; i++ {
			fpSrc.WriteString(newTokens[i])
		}
		fp := sha256HexBytes(fpSrc.Bytes())[:16]

		hunks = append(hunks, DiffHunk{
			ID:          id,
			Fingerprint: fp,
			Header:      header,
			OldStart:    oldLine1,
			OldLines:    oldCount,
			NewStart:    newLine1,
			NewLines:    newCount,
			Preview:     preview.String(),
		})
		spans = append(spans, hunkSpan{
			id:       id,
			oldStart: m.oldStart,
			oldEnd:   m.oldEnd,
			newStart: m.newStart,
			newEnd:   m.newEnd,
		})
	}

	return hunks, spans
}

func mergeSelectedHunkTokens(beforeBytes, proposedBytes []byte, spans []hunkSpan, selected map[string]bool) []byte {
	oldTokens := splitLinesKeepEnding(beforeBytes)
	newTokens := splitLinesKeepEnding(proposedBytes)

	var out bytes.Buffer
	out.Grow(len(proposedBytes))

	cursor := 0
	for _, sp := range spans {
		for i := cursor; i < sp.oldStart && i < len(oldTokens); i++ {
			out.WriteString(oldTokens[i])
		}
		if selected[sp.id] {
			for i := sp.newStart; i < sp.newEnd && i < len(newTokens); i++ {
				out.WriteString(newTokens[i])
			}
		} else {
			for i := sp.oldStart; i < sp.oldEnd && i < len(oldTokens); i++ {
				out.WriteString(oldTokens[i])
			}
		}
		cursor = sp.oldEnd
	}
	for i := cursor; i < len(oldTokens); i++ {
		out.WriteString(oldTokens[i])
	}
	return out.Bytes()
}

type changeRange struct {
	oldStart int
	oldEnd   int
	newStart int
	newEnd   int
}

func diffTokenRanges(a, b []string) []changeRange {
	n, m := len(a), len(b)
	pref := 0
	for pref < n && pref < m && a[pref] == b[pref] {
		pref++
	}
	suff := 0
	for suff < n-pref && suff < m-pref && a[n-1-suff] == b[m-1-suff] {
		suff++
	}

	aMid := a[pref : n-suff]
	bMid := b[pref : m-suff]
	if len(aMid) == 0 && len(bMid) == 0 {
		return nil
	}
	if len(aMid) == 0 {
		return []changeRange{{oldStart: pref, oldEnd: pref, newStart: pref, newEnd: pref + len(bMid)}}
	}
	if len(bMid) == 0 {
		return []changeRange{{oldStart: pref, oldEnd: pref + len(aMid), newStart: pref, newEnd: pref}}
	}

	if len(aMid)+len(bMid) > myersMaxMatrixOps {
		return []changeRange{{
			oldStart: pref,
			oldEnd:   pref + len(aMid),
			newStart: pref,
			newEnd:   pref + len(bMid),
		}}
	}

	midRanges := myersChanges(aMid, bMid)
	for i := range midRanges {
		midRanges[i].oldStart += pref
		midRanges[i].oldEnd += pref
		midRanges[i].newStart += pref
		midRanges[i].newEnd += pref
	}
	return midRanges
}

func myersChanges(a, b []string) []changeRange {
	n, m := len(a), len(b)
	maxD := n + m
	off := maxD
	v := make([]int, 2*maxD+1)
	var trace [][]int

	foundD := maxD
	for d := 0; d <= maxD; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		trace = append(trace, snap)

		done := false
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				foundD = d
				done = true
				break
			}
		}
		if done {
			break
		}
	}

	type editStep struct {
		op byte // '=', '-', '+'
	}
	var revSteps []editStep
	x, y := n, m
	for d := foundD; d > 0; d-- {
		prevV := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && prevV[off+k-1] < prevV[off+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := prevV[off+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			revSteps = append(revSteps, editStep{op: '='})
			x--
			y--
		}
		if prevK == k+1 {
			revSteps = append(revSteps, editStep{op: '+'})
			y--
		} else {
			revSteps = append(revSteps, editStep{op: '-'})
			x--
		}
	}
	for x > 0 && y > 0 {
		revSteps = append(revSteps, editStep{op: '='})
		x--
		y--
	}

	var ranges []changeRange
	oldIdx, newIdx := 0, 0
	inChange := false
	var cur changeRange

	for i := len(revSteps) - 1; i >= 0; i-- {
		switch revSteps[i].op {
		case '=':
			if inChange {
				cur.oldEnd = oldIdx
				cur.newEnd = newIdx
				ranges = append(ranges, cur)
				inChange = false
			}
			oldIdx++
			newIdx++
		case '-':
			if !inChange {
				cur = changeRange{oldStart: oldIdx, newStart: newIdx}
				inChange = true
			}
			oldIdx++
		case '+':
			if !inChange {
				cur = changeRange{oldStart: oldIdx, newStart: newIdx}
				inChange = true
			}
			newIdx++
		}
	}
	if inChange {
		cur.oldEnd = oldIdx
		cur.newEnd = newIdx
		ranges = append(ranges, cur)
	}
	return ranges
}

func splitLinesKeepEnding(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			out = append(out, string(data[start:i+1]))
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, string(data[start:]))
	}
	return out
}

func trimLineEnding(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
