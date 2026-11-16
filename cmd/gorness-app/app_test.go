package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/daemonctl"
	"gorness.dev/gorness/internal/store"
)

func TestEnrichDesktopPATHPrependsStandardBinsWithoutDuplicates(t *testing.T) {
	got := EnrichDesktopPATH("/usr/bin:/bin", "/Users/tester")
	parts := strings.Split(got, ":")
	wantPresent := []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/Users/tester/go/bin",
		"/Users/tester/.local/bin",
		"/usr/bin",
		"/bin",
	}
	for _, want := range wantPresent {
		found := 0
		for _, p := range parts {
			if p == want {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("expected %q once in %q, got count=%d", want, got, found)
		}
	}
}

func TestBundledDaemonBinaryResolvesSiblingEngine(t *testing.T) {
	exe := "/Applications/Gorness.app/Contents/MacOS/Gorness"
	got := BundledDaemonBinary(exe)
	want := filepath.Join("/Applications/Gorness.app/Contents/MacOS", "gorness-engine")
	if got != want {
		t.Fatalf("BundledDaemonBinary = %q, want %q", got, want)
	}
}

func TestIsAllowedLoopbackNavigationEnforcesDaemonOrigin(t *testing.T) {
	base := "http://127.0.0.1:3456"

	inApp, ext := IsAllowedLoopbackNavigation("http://127.0.0.1:3456/", base)
	if !inApp || ext {
		t.Fatalf("expected root loopback URL allowed in-app, got inApp=%v ext=%v", inApp, ext)
	}

	inApp, ext = IsAllowedLoopbackNavigation("http://127.0.0.1:3456/preview/tok_123", base)
	if !inApp || ext {
		t.Fatalf("expected /preview URL allowed in-app, got inApp=%v ext=%v", inApp, ext)
	}

	inApp, ext = IsAllowedLoopbackNavigation("http://127.0.0.1:9999/steal", base)
	if inApp || ext {
		t.Fatalf("expected cross-port loopback blocked, got inApp=%v ext=%v", inApp, ext)
	}

	inApp, ext = IsAllowedLoopbackNavigation("https://claude.com/docs", base)
	if inApp || !ext {
		t.Fatalf("expected external https to open externally, got inApp=%v ext=%v", inApp, ext)
	}

	inApp, ext = IsAllowedLoopbackNavigation("file:///etc/passwd", base)
	if inApp || ext {
		t.Fatalf("expected file:// URL blocked completely, got inApp=%v ext=%v", inApp, ext)
	}
}

func TestResolveVaultRootTildeExpansionAndDefaults(t *testing.T) {
	if got := ResolveVaultRoot(map[string]string{}, "/Users/alice"); got != "/Users/alice/.gorness" {
		t.Fatalf("default ResolveVaultRoot = %q, want /Users/alice/.gorness", got)
	}
	if got := ResolveVaultRoot(map[string]string{"GORNESS_VAULT": "~/custom-vault"}, "/Users/alice"); got != "/Users/alice/custom-vault" {
		t.Fatalf("tilde ResolveVaultRoot = %q, want /Users/alice/custom-vault", got)
	}
}

func TestRegisterProjectDirectoryAndPollAttentionOverUDS(t *testing.T) {
	shortDir, err := os.MkdirTemp("/tmp", "gapp-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(shortDir) })
	sockPath := filepath.Join(shortDir, "daemon.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req map[string]string
		_ = json.Unmarshal(raw, &req)
		_ = json.NewEncoder(w).Encode(store.Workspace{
			ID:       "ws_native_1",
			Name:     req["name"],
			RootPath: req["path"],
		})
	})
	mux.HandleFunc("/api/approvals", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]core.PendingApproval{
			{ID: "appr_1", ToolName: "run_command"},
			{ID: "appr_2", ToolName: "edit_file"},
		})
	})
	mux.HandleFunc("/api/alwayson", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(alwayson.AlwaysOnState{
			Running:       true,
			QueuedCount:   1,
			DeferredCount: 3,
		})
	})

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Listen unix: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	ep := &daemonctl.Endpoint{
		BaseURL:    daemonctl.UDSBaseURL,
		SocketPath: sockPath,
	}

	rawWS, err := registerProjectDirectory(context.Background(), ep, "", "/Users/alice/code/gorness-demo")
	if err != nil {
		t.Fatalf("registerProjectDirectory failed: %v", err)
	}
	var decodedWS store.Workspace
	if err := json.Unmarshal([]byte(rawWS), &decodedWS); err != nil {
		t.Fatalf("unmarshal workspace JSON %q: %v", rawWS, err)
	}
	if decodedWS.ID != "ws_native_1" || decodedWS.Name != "gorness-demo" {
		t.Fatalf("unexpected registered workspace: %+v", decodedWS)
	}

	js := FormatWorkspaceJSInvocation(rawWS)
	if !strings.Contains(js, "window.onNativeWorkspaceRegistered(") || !strings.Contains(js, `ws_native_1`) {
		t.Fatalf("unexpected FormatWorkspaceJSInvocation output: %s", js)
	}

	count, err := PollAttentionCount(context.Background(), ep, "")
	if err != nil {
		t.Fatalf("PollAttentionCount failed: %v", err)
	}
	if count != 5 {
		t.Fatalf("PollAttentionCount = %d, want 5 (2 approvals + 3 deferred)", count)
	}
	if title := FormatStatusBarTitle(0); title != "G·" {
		t.Fatalf("FormatStatusBarTitle(0) = %q, want 'G·'", title)
	}
	if title := FormatStatusBarTitle(count); title != "G·5" {
		t.Fatalf("FormatStatusBarTitle(5) = %q, want 'G·5'", title)
	}
}
