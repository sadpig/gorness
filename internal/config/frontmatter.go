package config

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ParseFrontmatterMD 解析支持 +++ (TOML) 与 --- (TOML 或 YAML 风格 key: value / 多行块标量 / 缩进子块) 头部元数据的 Markdown 文档。
func ParseFrontmatterMD(raw []byte) (meta map[string]any, body string, err error) {
	cleaned := bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	text := strings.ReplaceAll(string(cleaned), "\r\n", "\n")

	var delim string
	switch {
	case text == "+++" || strings.HasPrefix(text, "+++\n"):
		delim = "+++"
	case text == "---" || strings.HasPrefix(text, "---\n"):
		delim = "---"
	default:
		return map[string]any{}, text, nil
	}

	rest := strings.TrimPrefix(text, delim)
	rest = strings.TrimPrefix(rest, "\n")

	lines := strings.Split(rest, "\n")
	closeIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == delim {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return nil, "", fmt.Errorf("unclosed %s frontmatter header", delim)
	}

	header := strings.Join(lines[:closeIdx], "\n")
	if closeIdx+1 < len(lines) {
		body = strings.Join(lines[closeIdx+1:], "\n")
	}

	meta = make(map[string]any)
	if strings.TrimSpace(header) == "" {
		return meta, body, nil
	}

	if delim == "+++" {
		if err := toml.Unmarshal([]byte(header), &meta); err != nil {
			return nil, "", fmt.Errorf("parse +++ TOML frontmatter: %w", err)
		}
		return meta, body, nil
	}

	// --- 头优先尝试标准 TOML，若不匹配则解析兼容 SKILL.md / Agent MD 的 key: value 与 YAML 多行块语法。
	if err := toml.Unmarshal([]byte(header), &meta); err == nil {
		return meta, body, nil
	}
	kvMeta, kvErr := parseKeyValueHeader(header)
	if kvErr != nil {
		return nil, "", fmt.Errorf("parse --- frontmatter: %w", kvErr)
	}
	return kvMeta, body, nil
}

func parseKeyValueHeader(header string) (map[string]any, error) {
	return parseKeyValueLines(strings.Split(header, "\n"), 0)
}

func parseKeyValueLines(lines []string, baseLineOffset int) (map[string]any, error) {
	out := make(map[string]any)

	rootIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		rootIndent = countIndent(line)
		break
	}
	if rootIndent < 0 {
		return out, nil
	}

	for i := 0; i < len(lines); {
		rawLine := lines[i]
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		ind := countIndent(rawLine)
		if ind != rootIndent {
			return nil, fmt.Errorf("line %d: unexpected indentation", baseLineOffset+i+1)
		}

		sepIdx := strings.IndexAny(trimmed, ":=")
		if sepIdx <= 0 {
			return nil, fmt.Errorf("line %d: expected key: value or key = value", baseLineOffset+i+1)
		}
		key := strings.TrimSpace(trimmed[:sepIdx])
		if !isValidFrontmatterKey(key) {
			return nil, fmt.Errorf("line %d: invalid key %q", baseLineOffset+i+1, key)
		}

		sep := trimmed[sepIdx]
		valRaw := strings.TrimSpace(trimmed[sepIdx+1:])

		// 1. YAML 多行块标量（| / |- / |+ / > / >- / >+）
		if sep == ':' {
			if style, chomp, ok := parseYAMLBlockIndicator(valRaw); ok {
				j := i + 1
				for j < len(lines) {
					if strings.TrimSpace(lines[j]) == "" || countIndent(lines[j]) > rootIndent {
						j++
						continue
					}
					break
				}
				out[key] = formatYAMLBlockScalar(lines[i+1:j], style, chomp)
				i = j
				continue
			}
		}

		// 2. key: 后为空，向前探查列表项 (- item) 或缩进嵌套映射块
		if sep == ':' && valRaw == "" {
			peekIdx := nextMeaningfulLine(lines, i+1)
			if peekIdx < 0 {
				out[key] = ""
				i = len(lines)
				continue
			}
			peekInd := countIndent(lines[peekIdx])
			peekTrim := strings.TrimSpace(lines[peekIdx])

			if peekInd >= rootIndent && isYAMLListLine(peekTrim) {
				items, nextIdx, err := parseYAMLList(lines, i+1, peekInd, baseLineOffset)
				if err != nil {
					return nil, err
				}
				out[key] = items
				i = nextIdx
				continue
			}

			if peekInd > rootIndent {
				j := i + 1
				lastContentIdx := i + 1
				for j < len(lines) {
					t := strings.TrimSpace(lines[j])
					if t == "" || strings.HasPrefix(t, "#") {
						j++
						continue
					}
					if countIndent(lines[j]) > rootIndent {
						lastContentIdx = j + 1
						j++
						continue
					}
					break
				}
				nested, err := parseKeyValueLines(lines[i+1:lastContentIdx], baseLineOffset+i+1)
				if err != nil {
					return nil, err
				}
				out[key] = nested
				i = j
				continue
			}

			out[key] = ""
			i++
			continue
		}

		if valRaw == "" {
			return nil, fmt.Errorf("line %d: empty value for key %q", baseLineOffset+i+1, key)
		}

		// 3. 普通未加引号的 key: value 若后续行有更深缩进，按 YAML 多行折叠标量拼接
		if sep == ':' && !strings.HasPrefix(valRaw, "[") && !strings.HasPrefix(valRaw, "\"") && !strings.HasPrefix(valRaw, "'") {
			j := i + 1
			var parts []string
			hasContinuation := false
			for j < len(lines) {
				t := strings.TrimSpace(lines[j])
				if t == "" {
					nextIdx := nextMeaningfulLine(lines, j+1)
					if nextIdx >= 0 && countIndent(lines[nextIdx]) > rootIndent {
						j++
						continue
					}
					break
				}
				if countIndent(lines[j]) > rootIndent {
					if !strings.HasPrefix(t, "#") {
						if !hasContinuation {
							parts = append(parts, valRaw)
							hasContinuation = true
						}
						parts = append(parts, t)
					}
					j++
					continue
				}
				break
			}
			if hasContinuation {
				out[key] = strings.Join(parts, " ")
				i = j
				continue
			}
		}

		val, err := parseFrontmatterValue(valRaw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", baseLineOffset+i+1, err)
		}
		out[key] = val
		i++
	}

	return out, nil
}

func countIndent(line string) int {
	n := 0
	for _, r := range line {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 2
		default:
			return n
		}
	}
	return n
}

func stripIndentCols(line string, cols int) string {
	removed := 0
	idx := 0
	for idx < len(line) && removed < cols {
		switch line[idx] {
		case ' ':
			removed++
			idx++
		case '\t':
			removed += 2
			idx++
		default:
			return line[idx:]
		}
	}
	return line[idx:]
}

func nextMeaningfulLine(lines []string, start int) int {
	for idx := start; idx < len(lines); idx++ {
		t := strings.TrimSpace(lines[idx])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		return idx
	}
	return -1
}

func isYAMLListLine(trimmed string) bool {
	return trimmed == "-" || strings.HasPrefix(trimmed, "- ")
}

func parseYAMLList(lines []string, startIdx, listIndent, baseLineOffset int) ([]any, int, error) {
	items := []any{}
	i := startIdx
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			nextIdx := nextMeaningfulLine(lines, i+1)
			if nextIdx >= 0 && countIndent(lines[nextIdx]) >= listIndent {
				i++
				continue
			}
			break
		}
		ind := countIndent(lines[i])
		if ind < listIndent || !isYAMLListLine(t) {
			break
		}
		if ind > listIndent {
			return nil, i, fmt.Errorf("line %d: unexpected list item indentation", baseLineOffset+i+1)
		}

		itemRaw := strings.TrimSpace(strings.TrimPrefix(t, "-"))
		i++
		// 合并同一列表项下的缩进续行
		if itemRaw != "" && !strings.HasPrefix(itemRaw, "\"") && !strings.HasPrefix(itemRaw, "'") && !strings.HasPrefix(itemRaw, "[") {
			var cont []string
			for i < len(lines) {
				ct := strings.TrimSpace(lines[i])
				if ct == "" {
					break
				}
				if countIndent(lines[i]) > listIndent && !isYAMLListLine(ct) {
					if !strings.HasPrefix(ct, "#") {
						cont = append(cont, ct)
					}
					i++
					continue
				}
				break
			}
			if len(cont) > 0 {
				items = append(items, itemRaw+" "+strings.Join(cont, " "))
				continue
			}
		}

		if itemRaw == "" {
			items = append(items, "")
			continue
		}
		val, err := parseFrontmatterValue(itemRaw)
		if err != nil {
			return nil, i, fmt.Errorf("line %d: %w", baseLineOffset+i, err)
		}
		items = append(items, val)
	}
	return items, i, nil
}

func parseYAMLBlockIndicator(valRaw string) (style byte, chomp byte, ok bool) {
	tok := valRaw
	if idx := strings.Index(tok, " #"); idx >= 0 {
		tok = strings.TrimSpace(tok[:idx])
	}
	if len(tok) == 0 || (tok[0] != '|' && tok[0] != '>') {
		return 0, 0, false
	}
	style = tok[0]
	rest := tok[1:]
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		switch {
		case (c == '-' || c == '+') && chomp == 0:
			chomp = c
		case c >= '1' && c <= '9':
			// YAML 显式缩进宽度指示符（如 |2 / >-2）
		default:
			return 0, 0, false
		}
	}
	return style, chomp, true
}

func formatYAMLBlockScalar(blockLines []string, style byte, chomp byte) string {
	minIndent := -1
	for _, l := range blockLines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := countIndent(l)
		if minIndent < 0 || ind < minIndent {
			minIndent = ind
		}
	}
	if minIndent < 0 {
		return ""
	}

	dedented := make([]string, len(blockLines))
	for i, l := range blockLines {
		if strings.TrimSpace(l) == "" {
			dedented[i] = ""
		} else {
			dedented[i] = stripIndentCols(l, minIndent)
		}
	}

	var result string
	if style == '|' {
		result = strings.Join(dedented, "\n")
	} else {
		var b strings.Builder
		prevWasPlain := false
		pendingBlankCount := 0
		started := false

		for _, dl := range dedented {
			if dl == "" {
				if started {
					pendingBlankCount++
				}
				continue
			}
			moreIndented := countIndent(dl) > 0
			if !started {
				b.WriteString(dl)
				started = true
				prevWasPlain = !moreIndented
				continue
			}
			if pendingBlankCount > 0 {
				if prevWasPlain && !moreIndented {
					b.WriteString(strings.Repeat("\n", pendingBlankCount))
				} else {
					b.WriteString(strings.Repeat("\n", pendingBlankCount+1))
				}
				pendingBlankCount = 0
			} else if prevWasPlain && !moreIndented {
				b.WriteByte(' ')
			} else {
				b.WriteByte('\n')
			}
			b.WriteString(dl)
			prevWasPlain = !moreIndented
		}
		if pendingBlankCount > 0 && chomp == '+' {
			b.WriteString(strings.Repeat("\n", pendingBlankCount))
		}
		result = b.String()
	}

	if chomp == '+' {
		result = strings.TrimLeft(result, "\n")
		if result != "" && !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		return result
	}
	return strings.Trim(result, "\n")
}

func isValidFrontmatterKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func parseFrontmatterValue(raw string) (any, error) {
	if strings.HasPrefix(raw, "[") {
		if !strings.HasSuffix(raw, "]") {
			return nil, errors.New("unclosed array value")
		}
		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		if inner == "" {
			return []any{}, nil
		}
		parts := splitCommaRespectingQuotes(inner)
		arr := make([]any, 0, len(parts))
		for _, p := range parts {
			v, err := parseScalarValue(strings.TrimSpace(p))
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	}
	return parseScalarValue(raw)
}

func parseScalarValue(raw string) (any, error) {
	if len(raw) >= 2 && raw[0] == '"' {
		if raw[len(raw)-1] != '"' {
			return nil, errors.New("unterminated double-quoted string")
		}
		unq, err := strconv.Unquote(raw)
		if err != nil {
			return nil, err
		}
		return unq, nil
	}
	if strings.HasPrefix(raw, "\"") {
		return nil, errors.New("unterminated double-quoted string")
	}
	if len(raw) >= 2 && raw[0] == '\'' {
		if raw[len(raw)-1] != '\'' {
			return nil, errors.New("unterminated single-quoted string")
		}
		return raw[1 : len(raw)-1], nil
	}
	if strings.HasPrefix(raw, "'") {
		return nil, errors.New("unterminated single-quoted string")
	}

	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}
	if iv, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return iv, nil
	}
	if fv, err := strconv.ParseFloat(raw, 64); err == nil && strings.ContainsAny(raw, ".eE") {
		return fv, nil
	}
	return raw, nil
}

func splitCommaRespectingQuotes(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote == '"' {
			cur.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			cur.WriteRune(r)
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			cur.WriteRune(r)
			continue
		}
		if r == ',' {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	out = append(out, cur.String())
	return out
}
