package daemonctl

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/store"
)

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gctl-*")
	if err != nil {
		t.Fatalf("MkdirTemp /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestRuntimeRecordAtomicRoundTripAndVaultResolution(t *testing.T) {
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	// Verify ~/ expansion across both flag and GORNESS_VAULT environment variable.
	if got := ResolveVaultRoot("~/custom-vault", nil, homeDir); got != filepath.Join(homeDir, "custom-vault") {
		t.Fatalf("flag ~/ expansion got %q", got)
	}
	if got := ResolveVaultRoot("", map[string]string{"GORNESS_VAULT": "~/env-vault"}, homeDir); got != filepath.Join(homeDir, "env-vault") {
		t.Fatalf("env ~/ expansion got %q", got)
	}
	defaultVault := ResolveVaultRoot("", map[string]string{}, homeDir)
	if defaultVault != filepath.Join(homeDir, ".gorness") {
		t.Fatalf("default vault got %q", defaultVault)
	}

	paths := config.VaultPaths{Root: defaultVault}
	if gotLog := DaemonLogPath(paths); gotLog != filepath.Join(paths.LogsDir(), "daemon.log") {
		t.Fatalf("DaemonLogPath = %q", gotLog)
	}

	rec := RuntimeRecord{
		PID:        4242,
		Host:       "127.0.0.1",
		Port:       3456,
		URL:        "http://127.0.0.1:3456",
		SocketPath: paths.SocketPath(),
	}
	if err := WriteRuntimeRecord(paths, rec); err != nil {
		t.Fatalf("WriteRuntimeRecord: %v", err)
	}

	loaded, err := ReadRuntimeRecord(paths)
	if err != nil {
		t.Fatalf("ReadRuntimeRecord: %v", err)
	}
	if loaded.PID != 4242 || loaded.Version != config.Version || !VaultRootsMatch(loaded.VaultRoot, defaultVault) {
		t.Fatalf("unexpected loaded RuntimeRecord: %+v", loaded)
	}

	// Overlap check must catch both parent ($HOME) and child ($HOME/.gorness/workspaces).
	if !PathsOverlapVault(homeDir, defaultVault) {
		t.Fatal("expected $HOME to overlap $HOME/.gorness")
	}
	if !PathsOverlapVault(filepath.Join(defaultVault, "runtime"), defaultVault) {
		t.Fatal("expected child inside vault to overlap vault")
	}
	projectDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if PathsOverlapVault(projectDir, defaultVault) {
		t.Fatal("sibling project directory should not overlap vault")
	}

	// Enforce Zero-DB invariant: zero .json files inside vault.
	_ = filepath.WalkDir(defaultVault, func(p string, d os.DirEntry, wErr error) error {
		if wErr == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".json") {
			t.Fatalf("forbidden .json file in vault: %s", p)
		}
		return nil
	})

	if err := RemoveRuntimeRecord(paths); err != nil {
		t.Fatalf("RemoveRuntimeRecord: %v", err)
	}
}

func TestUDSPreferredTransportWithSeamlessTCPFallbackAndVaultIsolation(t *testing.T) {
	base := shortTempDir(t)
	vaultA := filepath.Join(base, "vaultA")
	vaultB := filepath.Join(base, "vaultB")
	pathsA := config.VaultPaths{Root: vaultA}
	if err := os.MkdirAll(pathsA.RuntimeDir(), 0o700); err != nil {
		t.Fatalf("mkdir runtimeA: %v", err)
	}
	if err := os.MkdirAll(vaultB, 0o700); err != nil {
		t.Fatalf("mkdir vaultB: %v", err)
	}

	var udsHits atomic.Int32
	var tcpHits atomic.Int32

	makeHandler := func(counter *atomic.Int32, servedVault string) http.Handler {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
			counter.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(HealthStatus{
				OK:        true,
				Version:   config.Version,
				PID:       os.Getpid(),
				VaultRoot: servedVault,
			})
		})
		mux.HandleFunc("POST /api/workspaces", func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			var in struct {
				Path string `json:"path"`
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			name := in.Name
			if name == "" {
				name = filepath.Base(in.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(store.Workspace{
				ID:       "ws_uds_1",
				Name:     name,
				RootPath: in.Path,
			})
		})
		return mux
	}

	tcpSrvA := httptest.NewServer(makeHandler(&tcpHits, vaultA))
	defer tcpSrvA.Close()

	sockPath := pathsA.SocketPath()
	unixLn, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}
	udsSrv := &http.Server{Handler: makeHandler(&udsHits, vaultA)}
	go func() { _ = udsSrv.Serve(unixLn) }()
	defer func() {
		_ = udsSrv.Close()
		_ = os.Remove(sockPath)
	}()

	if err := WriteRuntimeRecord(pathsA, RuntimeRecord{
		PID:        os.Getpid(),
		VaultRoot:  vaultA,
		URL:        tcpSrvA.URL,
		SocketPath: sockPath,
	}); err != nil {
		t.Fatalf("WriteRuntimeRecord: %v", err)
	}

	// 1. Probe vaultA -> must prefer UDS ("http://localhost") while preserving WebURL for browser/WKWebView.
	ep, ok := Probe(context.Background(), vaultA)
	if !ok || ep == nil {
		t.Fatal("expected Probe(vaultA) to succeed")
	}
	if ep.BaseURL != UDSBaseURL {
		t.Fatalf("expected UDS BaseURL %q, got %q", UDSBaseURL, ep.BaseURL)
	}
	if ep.WebURL != tcpSrvA.URL {
		t.Fatalf("expected WebURL %q, got %q", tcpSrvA.URL, ep.WebURL)
	}

	wsDir := filepath.Join(base, "my-project")
	_ = os.MkdirAll(wsDir, 0o755)
	ws, rawWS, err := RegisterWorkspace(context.Background(), ep, wsDir, "")
	if err != nil {
		t.Fatalf("RegisterWorkspace over UDS: %v", err)
	}
	if ws.ID != "ws_uds_1" || !strings.Contains(string(rawWS), `"my-project"`) {
		t.Fatalf("unexpected workspace response: ws=%+v raw=%s", ws, string(rawWS))
	}
	if udsHits.Load() < 2 || tcpHits.Load() != 0 {
		t.Fatalf("expected requests routed over UDS (uds=%d tcp=%d)", udsHits.Load(), tcpHits.Load())
	}

	// 2. Close UDS socket mid-flight -> existing Endpoint must fall back seamlessly to loopback TCP URL!
	_ = udsSrv.Close()
	_ = unixLn.Close()
	_ = os.Remove(sockPath)

	ws2, _, err := RegisterWorkspace(context.Background(), ep, wsDir, "FallbackCheck")
	if err != nil {
		t.Fatalf("expected seamless TCP fallback after UDS closed, got err: %v", err)
	}
	if ws2.Name != "FallbackCheck" || tcpHits.Load() == 0 {
		t.Fatalf("expected TCP fallback hit (tcpHits=%d ws2=%+v)", tcpHits.Load(), ws2)
	}

	// 3. Vault isolation: vaultB pointing its daemon.toml URL at vaultA's server MUST be rejected!
	pathsB := config.VaultPaths{Root: vaultB}
	_ = WriteRuntimeRecord(pathsB, RuntimeRecord{
		PID: os.Getpid(),
		URL: tcpSrvA.URL,
	})
	if badEP, badOK := Probe(context.Background(), vaultB); badOK {
		t.Fatalf("cross-vault hijack: Probe(vaultB) matched vaultA daemon %+v", badEP)
	}

	// StopDaemon must also refuse to signal a daemon.toml belonging to another vault.
	_ = WriteRuntimeRecord(pathsB, RuntimeRecord{
		PID:       999999,
		VaultRoot: vaultA,
		URL:       tcpSrvA.URL,
	})
	if _, _, stopErr := StopDaemon(context.Background(), vaultB, 200*time.Millisecond); !errors.Is(stopErr, ErrVaultMismatch) {
		t.Fatalf("expected ErrVaultMismatch from StopDaemon on cross-vault daemon.toml, got %v", stopErr)
	}
}

func TestEnsureDaemonSpawnsStarterAndWritesDaemonLog(t *testing.T) {
	base := shortTempDir(t)
	vaultDir := filepath.Join(base, "vault")
	paths := config.VaultPaths{Root: vaultDir}

	var srv *httptest.Server
	t.Cleanup(func() {
		if srv != nil {
			srv.Close()
		}
	})

	ep, err := EnsureDaemon(context.Background(), EnsureOptions{
		VaultRoot:    vaultDir,
		DaemonBinary: "/mock/gorness-engine",
		WaitTimeout:  2 * time.Second,
		Starter: func(bin string, args []string, logFile *os.File, _ []string) error {
			_, _ = logFile.WriteString("mock daemon starting: " + bin + " " + strings.Join(args, " ") + "\n")
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(HealthStatus{
					OK:        true,
					Version:   config.Version,
					PID:       54321,
					VaultRoot: vaultDir,
				})
			})
			srv = httptest.NewServer(mux)
			return WriteRuntimeRecord(paths, RuntimeRecord{
				PID:       54321,
				VaultRoot: vaultDir,
				URL:       srv.URL,
			})
		},
	})
	if err != nil {
		t.Fatalf("EnsureDaemon: %v", err)
	}
	if ep.WebURL != srv.URL || ep.Record.PID != 54321 {
		t.Fatalf("unexpected EnsureDaemon endpoint: %+v", ep)
	}

	logRaw, err := os.ReadFile(DaemonLogPath(paths))
	if err != nil || !strings.Contains(string(logRaw), "daemon serve --vault") {
		t.Fatalf("expected daemon.log output, got err=%v content=%q", err, string(logRaw))
	}
}
