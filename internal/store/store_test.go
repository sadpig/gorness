package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
)

type sharedCounterDoc struct {
	LastSeq uint64   `toml:"last_seq"`
	History []uint64 `toml:"history"`
}

func TestAtomicWriteAndFlockConcurrency(t *testing.T) {
	vaultRoot := filepath.Join(t.TempDir(), ".gorness")
	paths := config.VaultPaths{Root: vaultRoot}

	sessions, err := NewSessionStore(paths)
	if err != nil {
		t.Fatalf("NewSessionStore failed: %v", err)
	}

	lockPath := filepath.Join(paths.RuntimeDir(), "contend.lock")
	statePath := filepath.Join(paths.RuntimeDir(), "contend.toml")

	// Verify TryFlock non-blocking mutual exclusion before launching worker swarm.
	unlock1, ok, err := TryFlock(lockPath)
	if err != nil || !ok {
		t.Fatalf("expected first TryFlock to succeed: ok=%v err=%v", ok, err)
	}
	_, ok2, err := TryFlock(lockPath)
	if err != nil {
		t.Fatalf("second TryFlock returned unexpected error: %v", err)
	}
	if ok2 {
		t.Fatal("expected second TryFlock to report contention (ok=false) while lock is held")
	}
	// Idempotent unlock must release cleanly even if called twice.
	unlock1()
	unlock1()

	unlock3, ok3, err := TryFlock(lockPath)
	if err != nil || !ok3 {
		t.Fatalf("expected TryFlock after unlock to succeed: ok=%v err=%v", ok3, err)
	}
	unlock3()

	const workers = 20
	results := make(chan uint64, workers)
	errs := make(chan error, workers*2)
	startGate := make(chan struct{})

	// Concurrent reader verifying AtomicWriteFile never exposes a half-written/torn TOML file.
	stopReader := make(chan struct{})
	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for {
			select {
			case <-stopReader:
				return
			default:
				raw, rErr := os.ReadFile(statePath)
				if rErr != nil {
					if errors.Is(rErr, os.ErrNotExist) {
						continue
					}
					errs <- fmt.Errorf("concurrent read failed: %w", rErr)
					return
				}
				var doc sharedCounterDoc
				if uErr := toml.Unmarshal(raw, &doc); uErr != nil {
					errs <- fmt.Errorf("torn TOML file observed (%d bytes): %w", len(raw), uErr)
					return
				}
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startGate
			err := WithFlock(lockPath, true, func() error {
				seq, seqErr := sessions.NextSeq()
				if seqErr != nil {
					return seqErr
				}

				var doc sharedCounterDoc
				if raw, rErr := os.ReadFile(statePath); rErr == nil {
					if uErr := toml.Unmarshal(raw, &doc); uErr != nil {
						return fmt.Errorf("corrupt state inside flock: %w", uErr)
					}
				} else if !errors.Is(rErr, os.ErrNotExist) {
					return rErr
				}

				if seq <= doc.LastSeq {
					return fmt.Errorf("non-monotonic seq: got %d after %d", seq, doc.LastSeq)
				}
				doc.LastSeq = seq
				doc.History = append(doc.History, seq)

				encoded, mErr := toml.Marshal(doc)
				if mErr != nil {
					return mErr
				}
				if wErr := AtomicWriteFile(statePath, encoded, 0o600); wErr != nil {
					return wErr
				}
				results <- seq
				return nil
			})
			if err != nil {
				errs <- err
			}
		}()
	}

	close(startGate)
	wg.Wait()
	close(stopReader)
	readerWG.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrency error: %v", err)
	}

	got := make([]uint64, 0, workers)
	for seq := range results {
		got = append(got, seq)
	}
	if len(got) != workers {
		t.Fatalf("expected %d sequences, got %d", workers, len(got))
	}

	// Verify the recorded history inside contend.toml was strictly monotonic 1..20 in lock acquisition order.
	rawFinal, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read final state file: %v", err)
	}
	var finalDoc sharedCounterDoc
	if err := toml.Unmarshal(rawFinal, &finalDoc); err != nil {
		t.Fatalf("unmarshal final state file: %v", err)
	}
	if len(finalDoc.History) != workers {
		t.Fatalf("expected %d history entries, got %d", workers, len(finalDoc.History))
	}
	for i := 0; i < workers; i++ {
		want := uint64(i + 1)
		if finalDoc.History[i] != want {
			t.Fatalf("history[%d] = %d, want %d (full=%v)", i, finalDoc.History[i], want, finalDoc.History)
		}
	}

	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	for i, v := range got {
		if v != uint64(i+1) {
			t.Fatalf("sorted seq[%d] = %d, want %d", i, v, i+1)
		}
	}

	// Reopening SessionStore on the same vault must continue monotonically from 21.
	reopened, err := NewSessionStore(paths)
	if err != nil {
		t.Fatalf("reopen NewSessionStore failed: %v", err)
	}
	seq21, err := reopened.NextSeq()
	if err != nil {
		t.Fatalf("NextSeq after reopen failed: %v", err)
	}
	if seq21 != 21 {
		t.Fatalf("expected NextSeq after reopen = 21, got %d", seq21)
	}
}

func TestSessionJSONLTreeForkAndRewindWithFileSnapshots(t *testing.T) {
	baseTmp := t.TempDir()
	vaultRoot := filepath.Join(baseTmp, ".gorness")
	paths := config.VaultPaths{Root: vaultRoot}

	rawWsDir := filepath.Join(baseTmp, "real-repo")
	if err := os.MkdirAll(rawWsDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	symlinkWsDir := filepath.Join(baseTmp, "alias-repo")
	if err := os.Symlink(rawWsDir, symlinkWsDir); err != nil {
		t.Fatalf("symlink repo: %v", err)
	}

	wsStore, err := NewWorkspaceStore(paths)
	if err != nil {
		t.Fatalf("NewWorkspaceStore: %v", err)
	}
	ws, err := wsStore.Register(symlinkWsDir, "Core Engine")
	if err != nil {
		t.Fatalf("WorkspaceStore.Register: %v", err)
	}
	canonRealWs, err := filepath.EvalSymlinks(rawWsDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if ws.RootPath != canonRealWs {
		t.Fatalf("expected canonical RootPath %q, got %q", canonRealWs, ws.RootPath)
	}
	updatedWs, err := wsStore.Update(ws.ID, func(w *Workspace) {
		w.PermMode = "auto"
		w.RoleModels = map[string]string{"coder": "anthropic/claude-sonnet-5-5"}
	})
	if err != nil || updatedWs.PermMode != "auto" {
		t.Fatalf("WorkspaceStore.Update failed: ws=%+v err=%v", updatedWs, err)
	}
	if gotWs, err := wsStore.Get(ws.ID); err != nil || gotWs.RoleModels["coder"] != "anthropic/claude-sonnet-5-5" {
		t.Fatalf("WorkspaceStore.Get mismatch: ws=%+v err=%v", gotWs, err)
	}
	if len(wsStore.List()) != 1 {
		t.Fatalf("expected 1 registered workspace, got %d", len(wsStore.List()))
	}

	snaps, err := NewSnapshotStore(paths)
	if err != nil {
		t.Fatalf("NewSnapshotStore: %v", err)
	}
	sessions, err := NewSessionStore(paths)
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}

	// Prepare initial b.go before the session touches anything.
	bPath := filepath.Join(ws.RootPath, "b.go")
	initialBBytes := []byte("package main\n\nconst Version = \"v0-original\"\n// exact <bytes> & tabs\tpreserved\n")
	if err := os.WriteFile(bPath, initialBBytes, 0o644); err != nil {
		t.Fatalf("write initial b.go: %v", err)
	}

	meta, err := sessions.Create(ws.ID, "Implement Snapshot Engine", "coder")
	if err != nil {
		t.Fatalf("SessionStore.Create: %v", err)
	}

	// Seq 1: Assistant turn containing signed Thinking + CompactionData with <, >, & characters.
	wantThinking := "Step 1: verify invariants a < b && c > d before mutating workspace files."
	wantSignature := "sig_v1_ed25519_9f8e7d6c5b4a3210_exact_opaque_token=="
	wantCompaction := json.RawMessage(`{"anchor_seq":1,"decisions":["keep <zero-db> & flock"],"opaque":"+/=XYZ"}`)
	wantToolInput := json.RawMessage(`{"path":"b.go","expr":"x < 10 && y > 2"}`)

	e1, err := sessions.Append(meta.ID, SessionEntry{
		Role:      "assistant",
		ModelID:   "claude-opus-5-5",
		AgentRole: "lead",
		Blocks: []ContentBlock{
			{
				Type:      "thinking",
				Thinking:  wantThinking,
				Signature: wantSignature,
			},
			{
				Type:           "compaction",
				CompactionData: wantCompaction,
			},
			{
				Type:       "tool_use",
				ToolCallID: "call_01",
				ToolName:   "read_file",
				ToolInput:  wantToolInput,
			},
			{
				Type: "text",
				Text: "Analyzed repository baseline.",
			},
		},
		InputTokens:     120,
		OutputTokens:    80,
		CacheReadTokens: 400,
		CostUSD:         0.015,
	})
	if err != nil {
		t.Fatalf("Append seq 1: %v", err)
	}
	if e1.Seq != 1 {
		t.Fatalf("expected e1.Seq == 1, got %d", e1.Seq)
	}

	// Seq 2: Create a.go and record ChangeCreate at seq=2.
	aPath := filepath.Join(ws.RootPath, "a.go")
	_, aExistedBefore, err := snaps.CaptureFile(aPath)
	if err != nil || aExistedBefore {
		t.Fatalf("expected a.go not to exist before seq 2: exists=%v err=%v", aExistedBefore, err)
	}
	aBytes := []byte("package main\n\nfunc HelperA() string { return \"created-at-seq-2\" }\n")
	if err := os.WriteFile(aPath, aBytes, 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}
	aAfterHash, aExistsNow, err := snaps.CaptureFile(aPath)
	if err != nil || !aExistsNow {
		t.Fatalf("capture created a.go: exists=%v err=%v", aExistsNow, err)
	}

	e2, err := sessions.Append(meta.ID, SessionEntry{
		Role:      "assistant",
		ModelID:   "claude-sonnet-5-5",
		AgentRole: "coder",
		Blocks:    []ContentBlock{{Type: "text", Text: "Created a.go at seq 2."}},
	})
	if err != nil || e2.Seq != 2 {
		t.Fatalf("Append seq 2 failed: e2=%+v err=%v", e2, err)
	}
	if err := snaps.RecordStepChange(FileSnapshotRecord{
		Seq:       2,
		SessionID: meta.ID,
		RelPath:   "a.go",
		Kind:      ChangeCreate,
		AfterHash: aAfterHash,
	}); err != nil {
		t.Fatalf("RecordStepChange seq 2: %v", err)
	}

	// Seq 3: First edit to b.go (v0 -> v1) so we also verify reverse-Seq multi-edit rollback!
	bBeforeSeq3Hash, bExists, err := snaps.CaptureFile(bPath)
	if err != nil || !bExists {
		t.Fatalf("capture b.go before seq 3: exists=%v err=%v", bExists, err)
	}
	bV1Bytes := []byte("package main\n\nconst Version = \"v1-intermediate\"\n")
	if err := os.WriteFile(bPath, bV1Bytes, 0o644); err != nil {
		t.Fatalf("write b.go v1: %v", err)
	}
	bAfterSeq3Hash, _, err := snaps.CaptureFile(bPath)
	if err != nil {
		t.Fatalf("capture b.go after seq 3: %v", err)
	}
	e3, err := sessions.Append(meta.ID, SessionEntry{
		Role:      "user",
		AgentRole: "coder",
		Blocks:    []ContentBlock{{Type: "text", Text: "Intermediate update to b.go at seq 3."}},
	})
	if err != nil || e3.Seq != 3 {
		t.Fatalf("Append seq 3 failed: e3=%+v err=%v", e3, err)
	}
	if err := snaps.RecordStepChange(FileSnapshotRecord{
		Seq:        3,
		SessionID:  meta.ID,
		RelPath:    "b.go",
		Kind:       ChangeModify,
		BeforeHash: bBeforeSeq3Hash,
		AfterHash:  bAfterSeq3Hash,
	}); err != nil {
		t.Fatalf("RecordStepChange seq 3: %v", err)
	}

	// Seq 4: Second edit to b.go (v1 -> v2) recorded as ChangeModify at seq=4.
	bBeforeSeq4Hash, _, err := snaps.CaptureFile(bPath)
	if err != nil {
		t.Fatalf("capture b.go before seq 4: %v", err)
	}
	bV2Bytes := []byte("package main\n\nconst Version = \"v2-mutated-at-seq-4\"\n")
	if err := os.WriteFile(bPath, bV2Bytes, 0o644); err != nil {
		t.Fatalf("write b.go v2: %v", err)
	}
	bAfterSeq4Hash, _, err := snaps.CaptureFile(bPath)
	if err != nil {
		t.Fatalf("capture b.go after seq 4: %v", err)
	}
	e4, err := sessions.Append(meta.ID, SessionEntry{
		Role:      "assistant",
		ModelID:   "claude-sonnet-5-5",
		AgentRole: "coder",
		Blocks:    []ContentBlock{{Type: "text", Text: "Modified b.go at seq 4."}},
	})
	if err != nil || e4.Seq != 4 {
		t.Fatalf("Append seq 4 failed: e4=%+v err=%v", e4, err)
	}
	if err := snaps.RecordStepChange(FileSnapshotRecord{
		Seq:        4,
		SessionID:  meta.ID,
		RelPath:    "b.go",
		Kind:       ChangeModify,
		BeforeHash: bBeforeSeq4Hash,
		AfterHash:  bAfterSeq4Hash,
	}); err != nil {
		t.Fatalf("RecordStepChange seq 4: %v", err)
	}

	// Fork at seq=2 and verify complete branch isolation in both directions.
	forkMeta, err := sessions.Fork(meta.ID, 2, "Explore Alternative At Seq 2")
	if err != nil {
		t.Fatalf("Fork at seq 2 failed: %v", err)
	}
	if forkMeta.ParentSessionID != meta.ID || forkMeta.ForkedAtSeq != 2 || forkMeta.HeadSeq != 2 {
		t.Fatalf("unexpected forkMeta: %+v", forkMeta)
	}
	forkBranch, err := sessions.LoadActiveBranch(forkMeta.ID)
	if err != nil {
		t.Fatalf("LoadActiveBranch(fork): %v", err)
	}
	if len(forkBranch) != 2 || forkBranch[0].Seq != 1 || forkBranch[1].Seq != 2 {
		t.Fatalf("expected fork branch [1, 2], got %+v", forkBranch)
	}
	forkAppended, err := sessions.Append(forkMeta.ID, SessionEntry{
		Role:   "user",
		Blocks: []ContentBlock{{Type: "text", Text: "Divergent prompt only on forked branch"}},
	})
	if err != nil {
		t.Fatalf("Append to fork failed: %v", err)
	}
	origBeforeRewind, err := sessions.LoadActiveBranch(meta.ID)
	if err != nil || len(origBeforeRewind) != 4 {
		t.Fatalf("expected original session to still have 4 entries before rewind, got %d (err=%v)", len(origBeforeRewind), err)
	}

	// Rewind original session to seq=1 with restoreFiles=true.
	rewoundMeta, restoredFiles, err := sessions.Rewind(meta.ID, 1, true, snaps, symlinkWsDir)
	if err != nil {
		t.Fatalf("Rewind to seq 1 failed: %v", err)
	}
	if rewoundMeta.HeadSeq != 1 || rewoundMeta.HeadEntryID != e1.ID {
		t.Fatalf("expected rewound HeadSeq=1 HeadEntryID=%s, got %+v", e1.ID, rewoundMeta)
	}
	if len(restoredFiles) != 2 {
		t.Fatalf("expected 2 restored files (b.go and a.go), got %v", restoredFiles)
	}

	// Verify a.go is removed.
	if _, err := os.Stat(aPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected a.go to be removed after rewind to seq 1, stat err=%v", err)
	}

	// Verify b.go reverted to exact pre-edit v0 bytes (proving reverse-Seq 4 -> 3 rollback).
	gotBBytes, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatalf("read restored b.go: %v", err)
	}
	if !bytes.Equal(gotBBytes, initialBBytes) {
		t.Fatalf("b.go bytes mismatch after rewind:\ngot:  %q\nwant: %q", gotBBytes, initialBBytes)
	}

	// Verify LoadActiveBranch stops at seq=1 and preserves Thinking, Signature, ToolInput, and CompactionData byte-for-byte.
	rewoundBranch, err := sessions.LoadActiveBranch(meta.ID)
	if err != nil {
		t.Fatalf("LoadActiveBranch after rewind: %v", err)
	}
	if len(rewoundBranch) != 1 || rewoundBranch[0].Seq != 1 {
		t.Fatalf("expected active branch to stop at [seq=1], got %+v", rewoundBranch)
	}
	gotE1 := rewoundBranch[0]
	if gotE1.Blocks[0].Thinking != wantThinking || gotE1.Blocks[0].Signature != wantSignature {
		t.Fatalf("signed Thinking block altered:\ngot:  %+v\nwant: thinking=%q sig=%q", gotE1.Blocks[0], wantThinking, wantSignature)
	}
	if !bytes.Equal(gotE1.Blocks[1].CompactionData, wantCompaction) {
		t.Fatalf("CompactionData not preserved byte-for-byte:\ngot:  %s\nwant: %s", gotE1.Blocks[1].CompactionData, wantCompaction)
	}
	if !bytes.Equal(gotE1.Blocks[2].ToolInput, wantToolInput) {
		t.Fatalf("ToolInput not preserved byte-for-byte:\ngot:  %s\nwant: %s", gotE1.Blocks[2].ToolInput, wantToolInput)
	}

	// Forked session must remain completely unaffected by rewinding original session.
	forkBranchAfter, err := sessions.LoadActiveBranch(forkMeta.ID)
	if err != nil || len(forkBranchAfter) != 3 || forkBranchAfter[2].ID != forkAppended.ID {
		t.Fatalf("forked branch should remain intact at 3 entries, got %+v (err=%v)", forkBranchAfter, err)
	}

	// Appending to rewound session continues cleanly from e1 on a new branch in the tree.
	eNew, err := sessions.Append(meta.ID, SessionEntry{
		Role:   "user",
		Blocks: []ContentBlock{{Type: "text", Text: "Fresh continuation from seq 1"}},
	})
	if err != nil {
		t.Fatalf("Append after rewind failed: %v", err)
	}
	if eNew.ParentID != e1.ID {
		t.Fatalf("expected new entry ParentID=%q, got %q", e1.ID, eNew.ParentID)
	}
	branchContinued, err := sessions.LoadActiveBranch(meta.ID)
	if err != nil || len(branchContinued) != 2 || branchContinued[0].ID != e1.ID || branchContinued[1].ID != eNew.ID {
		t.Fatalf("unexpected continued branch: %+v (err=%v)", branchContinued, err)
	}

	// Security check: RestoreSessionAfterSeq must block path traversal, absolute paths, leaf symlinks,
	// intermediate escaping symlinks, and vault-pointing symlinks even if ledger JSONL is tampered on disk!
	outsideTarget := filepath.Join(baseTmp, "outside-secret.txt")
	if err := os.WriteFile(outsideTarget, []byte("DO_NOT_TOUCH"), 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	escapeLink := filepath.Join(ws.RootPath, "escape_dir")
	if err := os.Symlink(baseTmp, escapeLink); err != nil {
		t.Fatalf("create escape dir symlink: %v", err)
	}
	leafLink := filepath.Join(ws.RootPath, "leaf_escape.go")
	if err := os.Symlink(outsideTarget, leafLink); err != nil {
		t.Fatalf("create leaf symlink: %v", err)
	}
	vaultLink := filepath.Join(ws.RootPath, "vault_escape")
	if err := os.Symlink(paths.RuntimeDir(), vaultLink); err != nil {
		t.Fatalf("create vault symlink: %v", err)
	}
	evilHash, err := snaps.PutBlob([]byte("OVERWRITTEN"))
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	for idx, badRel := range []string{
		"../outside-secret.txt",
		"/etc/passwd",
		"escape_dir/outside-secret.txt",
		"leaf_escape.go",
		"vault_escape/seq.toml",
	} {
		subSess, cErr := sessions.Create(ws.ID, fmt.Sprintf("BadPath-%d", idx), "coder")
		if cErr != nil {
			t.Fatalf("create subSess: %v", cErr)
		}
		// Write directly to ledger JSONL to bypass RecordStepChange lexical check and test RestoreSessionAfterSeq directly.
		rawTampered, mErr := marshalJSONLine(FileSnapshotRecord{
			Seq:        900 + uint64(idx),
			SessionID:  subSess.ID,
			RelPath:    badRel,
			Kind:       ChangeModify,
			BeforeHash: evilHash,
		})
		if mErr != nil {
			t.Fatalf("marshal tampered record: %v", mErr)
		}
		if wErr := AtomicWriteFile(snaps.ledgerPath(subSess.ID), rawTampered, 0o600); wErr != nil {
			t.Fatalf("write tampered ledger: %v", wErr)
		}
		if _, rErr := snaps.RestoreSessionAfterSeq(subSess.ID, 1, ws.RootPath); !errors.Is(rErr, ErrInvalidPath) {
			t.Fatalf("expected RestoreSessionAfterSeq to reject %q with ErrInvalidPath, got %v", badRel, rErr)
		}
	}
	outsideBytes, err := os.ReadFile(outsideTarget)
	if err != nil || string(outsideBytes) != "DO_NOT_TOUCH" {
		t.Fatalf("outside file was modified or unreadable: %q err=%v", outsideBytes, err)
	}

	// Verify ChangeDelete rollback restores a deleted nested file byte-for-byte, and .meta.toml recovery after rewind.
	delSess, err := sessions.Create(ws.ID, "Delete Rollback & Meta Recovery", "coder")
	if err != nil {
		t.Fatalf("create delSess: %v", err)
	}
	delRel := "nested/keep_me.go"
	delAbs := filepath.Join(ws.RootPath, "nested", "keep_me.go")
	delOrigBytes := []byte("package nested\n\nconst Kept = true\n")
	if err := AtomicWriteFile(delAbs, delOrigBytes, 0o644); err != nil {
		t.Fatalf("write nested/keep_me.go: %v", err)
	}
	delBeforeHash, delExists, err := snaps.CaptureFile(delAbs)
	if err != nil || !delExists {
		t.Fatalf("capture nested/keep_me.go: exists=%v err=%v", delExists, err)
	}
	d1, err := sessions.Append(delSess.ID, SessionEntry{
		Role:   "user",
		Blocks: []ContentBlock{{Type: "text", Text: "Keep baseline at d1"}},
	})
	if err != nil {
		t.Fatalf("Append d1: %v", err)
	}
	d2, err := sessions.Append(delSess.ID, SessionEntry{
		Role:   "assistant",
		Blocks: []ContentBlock{{Type: "text", Text: "UniqueDeadBranchTokenXYZ987 deleted nested/keep_me.go"}},
	})
	if err != nil {
		t.Fatalf("Append d2: %v", err)
	}
	if err := os.Remove(delAbs); err != nil {
		t.Fatalf("remove nested/keep_me.go: %v", err)
	}
	if err := snaps.RecordStepChange(FileSnapshotRecord{
		Seq:        d2.Seq,
		SessionID:  delSess.ID,
		RelPath:    delRel,
		Kind:       ChangeDelete,
		BeforeHash: delBeforeHash,
	}); err != nil {
		t.Fatalf("RecordStepChange ChangeDelete: %v", err)
	}
	if _, _, err := sessions.Rewind(delSess.ID, d1.Seq, true, snaps, ws.RootPath); err != nil {
		t.Fatalf("Rewind ChangeDelete session: %v", err)
	}
	restoredDelBytes, err := os.ReadFile(delAbs)
	if err != nil || !bytes.Equal(restoredDelBytes, delOrigBytes) {
		t.Fatalf("expected deleted file restored byte-for-byte, got %q (err=%v)", restoredDelBytes, err)
	}
	// Dead rewound branch must NOT pollute Search results.
	if deadHits, sErr := sessions.Search("UniqueDeadBranchTokenXYZ987", 10); sErr != nil || len(deadHits) != 0 {
		t.Fatalf("expected 0 search hits for rewound dead branch token, got %+v (err=%v)", deadHits, sErr)
	}
	// Remove .meta.toml and verify NewSessionStore reconstructs exact rewound HeadEntryID from .jsonl audit log!
	if err := os.Remove(sessions.sessionMetaPath(ws.ID, delSess.ID)); err != nil {
		t.Fatalf("remove delSess meta.toml: %v", err)
	}
	reloadedSessions, err := NewSessionStore(paths)
	if err != nil {
		t.Fatalf("NewSessionStore reload: %v", err)
	}
	recoveredMeta, err := reloadedSessions.GetMeta(delSess.ID)
	if err != nil {
		t.Fatalf("GetMeta recovered delSess: %v", err)
	}
	if recoveredMeta.HeadEntryID != d1.ID || recoveredMeta.HeadSeq != d1.Seq {
		t.Fatalf("expected recovered HeadEntryID=%s HeadSeq=%d, got %+v", d1.ID, d1.Seq, recoveredMeta)
	}
}

func TestCrossWorkspaceSearchAndExports(t *testing.T) {
	baseTmp := t.TempDir()
	vaultRoot := filepath.Join(baseTmp, ".gorness")
	paths := config.VaultPaths{Root: vaultRoot}

	wsDirA := filepath.Join(baseTmp, "project-alpha")
	wsDirB := filepath.Join(baseTmp, "project-beta")
	if err := os.MkdirAll(wsDirA, 0o755); err != nil {
		t.Fatalf("mkdir wsA: %v", err)
	}
	if err := os.MkdirAll(wsDirB, 0o755); err != nil {
		t.Fatalf("mkdir wsB: %v", err)
	}

	wsStore, err := NewWorkspaceStore(paths)
	if err != nil {
		t.Fatalf("NewWorkspaceStore: %v", err)
	}
	wsA, err := wsStore.Register(wsDirA, "Alpha")
	if err != nil {
		t.Fatalf("Register wsA: %v", err)
	}
	wsB, err := wsStore.Register(wsDirB, "Beta")
	if err != nil {
		t.Fatalf("Register wsB: %v", err)
	}

	sessions, err := NewSessionStore(paths)
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}

	// Session A1 (Workspace A): Title matches "零数据库文件锁" and body mentions WithFlock.
	sA1, err := sessions.Create(wsA.ID, "零数据库文件锁架构设计与压测", "lead")
	if err != nil {
		t.Fatalf("Create sA1: %v", err)
	}
	if _, err := sessions.Append(sA1.ID, SessionEntry{
		Role:      "user",
		AgentRole: "lead",
		Blocks: []ContentBlock{
			{Type: "text", Text: "请使用 syscall.Flock 实现跨进程排他锁 WithFlock 并验证高并发原子写入。"},
		},
	}); err != nil {
		t.Fatalf("Append sA1: %v", err)
	}

	// Session B1 (Workspace B): Title is generic, body mentions "零数据库文件锁" once and includes RestoreSessionAfterSeq + XSS/Bidi vectors.
	sB1, err := sessions.Create(wsB.ID, "Backend Storage Refactoring Notes", "coder")
	if err != nil {
		t.Fatalf("Create sB1: %v", err)
	}
	if _, err := sessions.Append(sB1.ID, SessionEntry{
		Role:      "user",
		AgentRole: "coder",
		Blocks: []ContentBlock{
			{
				Type: "text",
				Text: "Audit RestoreSessionAfterSeq against symlink escapes, 零数据库文件锁 edge cases, Trojan\u202eSource, and <script>alert('xss')</script><img src=x onerror=alert(1)>.",
			},
		},
	}); err != nil {
		t.Fatalf("Append sB1 user: %v", err)
	}
	if _, err := sessions.Append(sB1.ID, SessionEntry{
		Role:      "assistant",
		ModelID:   "claude-sonnet-5-5",
		AgentRole: "coder",
		Blocks: []ContentBlock{
			{
				Type:      "thinking",
				Thinking:  "Checking canonical root path via filepath.EvalSymlinks and <tag> escaping.",
				Signature: "sig_export_test",
			},
			{
				Type:       "tool_use",
				ToolCallID: "call_export_1",
				ToolName:   "search_code",
				ToolInput:  json.RawMessage(`{"pattern":"RestoreSessionAfterSeq"}`),
			},
			{
				Type: "text",
				Text: "Verified `RestoreSessionAfterSeq` rolls back mutations in reverse sequence order.",
			},
		},
		InputTokens:  250,
		OutputTokens: 140,
		CostUSD:      0.0042,
	}); err != nil {
		t.Fatalf("Append sB1 assistant: %v", err)
	}

	// Session B2 (Workspace B): Unrelated topic + archived check.
	sB2, err := sessions.Create(wsB.ID, "Apple HIG Typography Polish", "coder")
	if err != nil {
		t.Fatalf("Create sB2: %v", err)
	}
	if _, err := sessions.Append(sB2.ID, SessionEntry{
		Role:   "user",
		Blocks: []ContentBlock{{Type: "text", Text: "Tune SF Pro optical sizing and translucent sidebar vibrancy."}},
	}); err != nil {
		t.Fatalf("Append sB2: %v", err)
	}

	// 1. Chinese query: both sA1 (title hit) and sB1 (body hit) match across 2 workspaces, with sA1 ranked #1!
	zhHits, err := sessions.Search("零数据库文件锁", 10)
	if err != nil {
		t.Fatalf("Search Chinese failed: %v", err)
	}
	if len(zhHits) != 2 {
		t.Fatalf("expected 2 hits for '零数据库文件锁', got %d: %+v", len(zhHits), zhHits)
	}
	if zhHits[0].SessionID != sA1.ID || zhHits[0].WorkspaceID != wsA.ID {
		t.Fatalf("expected title hit sA1 ranked first, got %+v", zhHits)
	}
	if zhHits[1].SessionID != sB1.ID || zhHits[1].WorkspaceID != wsB.ID {
		t.Fatalf("expected body hit sB1 ranked second, got %+v", zhHits)
	}
	if zhHits[0].Score <= zhHits[1].Score {
		t.Fatalf("expected title match score (%f) > body-only match score (%f)", zhHits[0].Score, zhHits[1].Score)
	}

	// 2. Code identifier query: exact symbol RestoreSessionAfterSeq matches only sB1.
	codeHits, err := sessions.Search("RestoreSessionAfterSeq", 10)
	if err != nil {
		t.Fatalf("Search code identifier failed: %v", err)
	}
	if len(codeHits) != 1 || codeHits[0].SessionID != sB1.ID {
		t.Fatalf("expected exact 1 hit on sB1 for RestoreSessionAfterSeq, got %+v", codeHits)
	}
	if strings.ContainsRune(codeHits[0].Snippet, '\u202e') {
		t.Fatalf("expected snippet to escape Bidi control character, got %q", codeHits[0].Snippet)
	}

	// 3. English title query: "Typography Polish" ranks sB2 #1.
	enHits, err := sessions.Search("Typography Polish", 5)
	if err != nil {
		t.Fatalf("Search English failed: %v", err)
	}
	if len(enHits) != 1 || enHits[0].SessionID != sB2.ID {
		t.Fatalf("expected sB2 for 'Typography Polish', got %+v", enHits)
	}

	// 4. Verify ExportMarkdown is self-contained, Bidi-safe, and includes metadata, thinking, tool calls, and text.
	mdOut, err := sessions.ExportMarkdown(sB1.ID)
	if err != nil {
		t.Fatalf("ExportMarkdown failed: %v", err)
	}
	for _, mustContain := range []string{
		"# Backend Storage Refactoring Notes",
		sB1.ID,
		"RestoreSessionAfterSeq",
		"search_code",
		"<U+202E>",
	} {
		if !strings.Contains(mdOut, mustContain) {
			t.Fatalf("ExportMarkdown missing %q in output:\n%s", mustContain, mdOut)
		}
	}
	if strings.ContainsRune(mdOut, '\u202e') {
		t.Fatal("ExportMarkdown leaked raw Unicode Bidi control rune")
	}

	// 5. Verify ExportHTML is self-contained (<style>, <!DOCTYPE html>), Bidi-safe, and completely XSS-escaped.
	htmlOut, err := sessions.ExportHTML(sB1.ID)
	if err != nil {
		t.Fatalf("ExportHTML failed: %v", err)
	}
	for _, mustContain := range []string{
		"<!DOCTYPE html>",
		"<style>",
		"Backend Storage Refactoring Notes",
		"RestoreSessionAfterSeq",
		"&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		"&lt;U+202E&gt;",
	} {
		if !strings.Contains(htmlOut, mustContain) {
			t.Fatalf("ExportHTML missing %q in output:\n%s", mustContain, htmlOut)
		}
	}
	if strings.Contains(htmlOut, "<script>alert") || strings.Contains(htmlOut, "<img src=x onerror=") {
		t.Fatalf("ExportHTML contains unescaped XSS payload:\n%s", htmlOut)
	}
	if strings.ContainsRune(htmlOut, '\u202e') {
		t.Fatal("ExportHTML leaked raw Unicode Bidi control rune")
	}

	// 6. Verify UpdateMeta (pin/archive/draft), MoveWorkspace, and Delete lifecycle.
	if _, err := sessions.UpdateMeta(sB2.ID, func(m *SessionMeta) {
		m.Archived = true
		m.Draft = "unsent draft text"
	}); err != nil {
		t.Fatalf("UpdateMeta archive: %v", err)
	}
	if gotActive := sessions.List(wsB.ID, false); len(gotActive) != 1 || gotActive[0].ID != sB1.ID {
		t.Fatalf("List(includeArchived=false) expected only sB1, got %+v", gotActive)
	}
	if gotAll := sessions.List(wsB.ID, true); len(gotAll) != 2 {
		t.Fatalf("List(includeArchived=true) expected 2 sessions, got %d", len(gotAll))
	}

	moved, err := sessions.MoveWorkspace(sB1.ID, wsA.ID)
	if err != nil || moved.WorkspaceID != wsA.ID {
		t.Fatalf("MoveWorkspace failed: moved=%+v err=%v", moved, err)
	}
	if gotWsA := sessions.List(wsA.ID, false); len(gotWsA) != 2 {
		t.Fatalf("expected 2 active sessions in wsA after move, got %d", len(gotWsA))
	}

	if err := sessions.Delete(sB2.ID); err != nil {
		t.Fatalf("Delete sB2: %v", err)
	}
	if _, err := sessions.GetMeta(sB2.ID); err == nil {
		t.Fatal("expected GetMeta on deleted session to fail")
	}
}

func TestWorkspaceRegisterRejectsVaultContainingRoot(t *testing.T) {
	base := t.TempDir()
	homeDir := filepath.Join(base, "home")
	vaultRoot := filepath.Join(homeDir, ".gorness")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	wsStore, err := NewWorkspaceStore(config.VaultPaths{Root: vaultRoot})
	if err != nil {
		t.Fatalf("NewWorkspaceStore: %v", err)
	}

	// Registering $HOME (which contains ~/.gorness) or its parent MUST fail with ErrInvalidPath.
	if _, err := wsStore.Register(homeDir, "HomeDir"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath when registering vault-containing $HOME, got %v", err)
	}
	if _, err := wsStore.Register(base, "BaseParent"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath when registering parent of vault, got %v", err)
	}
	if _, err := wsStore.Register(vaultRoot, "VaultSelf"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath when registering vault itself, got %v", err)
	}

	// Sibling project directory succeeds cleanly.
	siblingProj := filepath.Join(homeDir, "projects", "demo")
	if err := os.MkdirAll(siblingProj, 0o755); err != nil {
		t.Fatalf("mkdir siblingProj: %v", err)
	}
	if ws, err := wsStore.Register(siblingProj, "Demo"); err != nil || ws == nil {
		t.Fatalf("expected sibling project registration to succeed, got ws=%+v err=%v", ws, err)
	}
}

func TestSlice1ArtifactsSnapshotGCIndexAndVaultSearch(t *testing.T) {
	vaultRoot := filepath.Join(t.TempDir(), ".gorness")
	paths := config.VaultPaths{Root: vaultRoot}

	// 1. Exported atomic/search helpers
	if err := ValidateSafeID("valid_id-01"); err != nil {
		t.Fatalf("ValidateSafeID rejected valid ID: %v", err)
	}
	if err := ValidateSafeID("../escape"); err == nil {
		t.Fatal("ValidateSafeID accepted path traversal")
	}
	if hexStr := RandomHex(8); len(hexStr) != 16 {
		t.Fatalf("expected RandomHex(8) length 16, got %q", hexStr)
	}
	lineBytes, err := MarshalJSONLine(map[string]string{"msg": "hello\nworld"})
	if err != nil || bytes.Count(lineBytes, []byte("\n")) != 1 {
		t.Fatalf("MarshalJSONLine failed: %q err=%v", string(lineBytes), err)
	}
	jsonlPath := filepath.Join(paths.LogsDir(), "events.jsonl")
	if err := AppendJSONLine(jsonlPath, map[string]int{"seq": 1}); err != nil {
		t.Fatalf("AppendJSONLine: %v", err)
	}
	tokens := TokenizeQuery("RaftConsensus Quorum_Lease 42")
	if len(tokens) < 3 {
		t.Fatalf("expected TokenizeQuery tokens, got %v", tokens)
	}

	wsStore, err := NewWorkspaceStore(paths)
	if err != nil {
		t.Fatalf("NewWorkspaceStore: %v", err)
	}
	repoDir := filepath.Join(t.TempDir(), "kernel-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repoDir: %v", err)
	}
	ws, err := wsStore.Register(repoDir, "KernelRepo")
	if err != nil {
		t.Fatalf("Register workspace: %v", err)
	}

	// 2. ArtifactStore CRUD (.html & .md, index.toml, zero .json files)
	artStore, err := NewArtifactStore(paths)
	if err != nil {
		t.Fatalf("NewArtifactStore: %v", err)
	}
	savedHTML, err := artStore.Save(ws.ID, Artifact{
		ID:        "flamegraph-report",
		SessionID: "sess_demo",
		Title:     "CPU Flamegraph Interactive View",
		Format:    "html",
		Content:   "<!doctype html><html><body><h1>Flamegraph</h1></body></html>",
	})
	if err != nil {
		t.Fatalf("ArtifactStore.Save html: %v", err)
	}
	savedMD, err := artStore.Save(ws.ID, Artifact{
		ID:      "latency-audit",
		Title:   "Tail Latency Root Cause",
		Format:  "md",
		Content: "# Tail Latency\nUniqueTokenArtifactZephyr99 in p99 scheduler queue.\n",
	})
	if err != nil {
		t.Fatalf("ArtifactStore.Save md: %v", err)
	}
	listedArts, err := artStore.List(ws.ID)
	if err != nil || len(listedArts) != 2 {
		t.Fatalf("ArtifactStore.List expected 2, got %+v err=%v", listedArts, err)
	}
	gotHTML, err := artStore.Get(ws.ID, savedHTML.ID)
	if err != nil || !strings.Contains(gotHTML.Content, "<h1>Flamegraph</h1>") {
		t.Fatalf("ArtifactStore.Get html failed: got=%+v err=%v", gotHTML, err)
	}

	// 3. SnapshotStore ListSessionRecords, DeleteSessionSnapshots & blob GC on SessionStore.Delete
	snaps, err := NewSnapshotStore(paths)
	if err != nil {
		t.Fatalf("NewSnapshotStore: %v", err)
	}
	sessions, err := NewSessionStore(paths)
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	s1, err := sessions.Create(ws.ID, "Ephemeral Session", "coder")
	if err != nil {
		t.Fatalf("Create s1: %v", err)
	}
	s2, err := sessions.Create(ws.ID, "Surviving Session", "coder")
	if err != nil {
		t.Fatalf("Create s2: %v", err)
	}

	uniqueFile := filepath.Join(repoDir, "unique_s1.go")
	sharedFile := filepath.Join(repoDir, "shared_both.go")
	if err := os.WriteFile(uniqueFile, []byte("package main // exclusive_s1_blob_payload\n"), 0o644); err != nil {
		t.Fatalf("write uniqueFile: %v", err)
	}
	if err := os.WriteFile(sharedFile, []byte("package main // shared_s1_s2_blob_payload\n"), 0o644); err != nil {
		t.Fatalf("write sharedFile: %v", err)
	}

	uniqueHash, _, err := snaps.CaptureFile(uniqueFile)
	if err != nil {
		t.Fatalf("CaptureFile unique: %v", err)
	}
	sharedHash, _, err := snaps.CaptureFile(sharedFile)
	if err != nil {
		t.Fatalf("CaptureFile shared: %v", err)
	}
	if err := snaps.RecordStepChange(FileSnapshotRecord{
		SessionID:  s1.ID,
		Seq:        1,
		RelPath:    "unique_s1.go",
		Kind:       ChangeModify,
		BeforeHash: uniqueHash,
	}); err != nil {
		t.Fatalf("RecordStepChange unique s1: %v", err)
	}
	if err := snaps.RecordStepChange(FileSnapshotRecord{
		SessionID:  s1.ID,
		Seq:        2,
		RelPath:    "shared_both.go",
		Kind:       ChangeModify,
		BeforeHash: sharedHash,
	}); err != nil {
		t.Fatalf("RecordStepChange shared s1: %v", err)
	}
	if err := snaps.RecordStepChange(FileSnapshotRecord{
		SessionID:  s2.ID,
		Seq:        1,
		RelPath:    "shared_both.go",
		Kind:       ChangeModify,
		BeforeHash: sharedHash,
	}); err != nil {
		t.Fatalf("RecordStepChange shared s2: %v", err)
	}

	s1Recs, err := snaps.ListSessionRecords(ws.ID, s1.ID)
	if err != nil || len(s1Recs) != 2 {
		t.Fatalf("ListSessionRecords s1 expected 2, got %+v err=%v", s1Recs, err)
	}

	// Deleting s1 via SessionStore.Delete MUST purge s1's snapshot ledger and GC uniqueHash's blob,
	// while keeping sharedHash's blob alive because s2 still references it!
	if err := sessions.Delete(s1.ID); err != nil {
		t.Fatalf("sessions.Delete(s1): %v", err)
	}
	afterRecs, err := snaps.ListSessionRecords(ws.ID, s1.ID)
	if err != nil || len(afterRecs) != 0 {
		t.Fatalf("expected 0 snapshot records after session delete, got %+v err=%v", afterRecs, err)
	}
	uniqueBlobPath := filepath.Join(paths.SnapshotsDir(), "blobs", uniqueHash[:2], uniqueHash)
	sharedBlobPath := filepath.Join(paths.SnapshotsDir(), "blobs", sharedHash[:2], sharedHash)
	if _, err := os.Stat(uniqueBlobPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected unreferenced blob %s to be garbage-collected, stat err=%v", uniqueBlobPath, err)
	}
	if _, err := os.Stat(sharedBlobPath); err != nil {
		t.Fatalf("expected shared blob %s referenced by s2 to remain intact, stat err=%v", sharedBlobPath, err)
	}

	// 4. Vault-wide BM25 Search across sessions, memory/notes/*.md, workspaces/<wsID>/memory/progress/*.md, and artifacts/*.md
	if _, err := sessions.Append(s2.ID, SessionEntry{
		Role:   "user",
		Blocks: []ContentBlock{{Type: "text", Text: "Investigate UniqueTokenSessionQuasar77 lock contention"}},
	}); err != nil {
		t.Fatalf("Append s2: %v", err)
	}
	notesDir := filepath.Join(paths.MemoryDir(), "notes")
	if err := os.MkdirAll(notesDir, 0o700); err != nil {
		t.Fatalf("mkdir notesDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "raft-lease.md"), []byte("# Raft Lease Note\nUniqueTokenNoteNebula88 prevents split-brain reads.\n"), 0o600); err != nil {
		t.Fatalf("write note: %v", err)
	}
	progDir := filepath.Join(paths.WorkspacesDir(), ws.ID, "memory", "progress")
	if err := os.MkdirAll(progDir, 0o700); err != nil {
		t.Fatalf("mkdir progDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(progDir, "sprint-1.md"), []byte("# Sprint Progress\nCompleted UniqueTokenProgressPulsar66 milestone.\n"), 0o600); err != nil {
		t.Fatalf("write progress: %v", err)
	}

	for _, tc := range []struct {
		query string
		desc  string
	}{
		{"UniqueTokenSessionQuasar77", "session transcript"},
		{"UniqueTokenNoteNebula88", "global memory note"},
		{"UniqueTokenProgressPulsar66", "workspace progress markdown"},
		{"UniqueTokenArtifactZephyr99", "workspace markdown artifact"},
	} {
		hits, err := sessions.Search(tc.query, 10)
		if err != nil || len(hits) == 0 {
			t.Fatalf("Search(%q) for %s expected >=1 hit, got %+v err=%v", tc.query, tc.desc, hits, err)
		}
	}

	if err := artStore.Delete(ws.ID, savedMD.ID); err != nil {
		t.Fatalf("ArtifactStore.Delete: %v", err)
	}

	// Verify ZERO .json files anywhere inside vaultRoot!
	if err := filepath.WalkDir(vaultRoot, func(p string, d os.DirEntry, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return fmt.Errorf("forbidden .json file inside vault: %s", p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
