package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/config"
)

type daemonRuntimeRecord struct {
	PID       int    `toml:"pid"`
	URL       string `toml:"url"`
	VaultRoot string `toml:"vault_root"`
}

func ResolveVaultRoot(env map[string]string, homeDir string) string {
	if v := strings.TrimSpace(env["GORNESS_VAULT"]); v != "" {
		if strings.HasPrefix(v, "~/") {
			return filepath.Join(homeDir, v[2:])
		}
		return filepath.Clean(v)
	}
	return filepath.Join(homeDir, ".gorness")
}

// EnrichDesktopPATH 补齐 Finder/Dock 启动 .app 时缺失的常用开发工具链目录，确保沙箱内 git/go/node/rg 可用。
func EnrichDesktopPATH(currentPATH, homeDir string) string {
	priority := []string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
	}
	if strings.TrimSpace(homeDir) != "" {
		priority = append(priority,
			filepath.Join(homeDir, "go", "bin"),
			filepath.Join(homeDir, ".local", "bin"),
		)
	}
	priority = append(priority, "/usr/bin", "/bin", "/usr/sbin", "/sbin")

	seen := make(map[string]bool, len(priority)+8)
	var out []string
	add := func(dir string) {
		d := strings.TrimSpace(dir)
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	for _, p := range priority {
		add(p)
	}
	for _, p := range strings.Split(currentPATH, ":") {
		add(p)
	}
	return strings.Join(out, ":")
}

func BundledDaemonBinary(appExecutablePath string) string {
	dir := filepath.Dir(appExecutablePath)
	return filepath.Join(dir, "gorness-engine")
}

// IsAllowedLoopbackNavigation 限制 WKWebView 严格停留在本地守护进程同一 Origin；外链仅允许交由系统浏览器打开。
func IsAllowedLoopbackNavigation(targetURL, daemonBaseURL string) (inApp bool, openExternal bool) {
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || u.Scheme == "" {
		return false, false
	}
	base, err := url.Parse(strings.TrimSpace(daemonBaseURL))
	if err != nil || base.Host == "" {
		return false, false
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == strings.ToLower(base.Scheme) && strings.EqualFold(u.Host, base.Host) {
		return true, false
	}

	if scheme == "http" || scheme == "https" || scheme == "mailto" {
		host, _, splitErr := net.SplitHostPort(u.Host)
		if splitErr != nil {
			host = u.Host
		}
		if host == "127.0.0.1" || strings.EqualFold(host, "localhost") || host == "::1" {
			return false, false
		}
		return false, true
	}
	return false, false
}

func canonicalPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
}

func probeHealthyDaemon(vaultRoot string) string {
	paths := config.VaultPaths{Root: vaultRoot}
	var candidates []string
	infoFile := filepath.Join(paths.RuntimeDir(), "daemon.toml")
	if raw, err := os.ReadFile(infoFile); err == nil {
		var rec daemonRuntimeRecord
		if toml.Unmarshal(raw, &rec) == nil && strings.TrimSpace(rec.URL) != "" {
			candidates = append(candidates, strings.TrimRight(rec.URL, "/"))
		}
	}
	candidates = append(candidates, "http://127.0.0.1:3456")

	wantVault := canonicalPath(vaultRoot)
	client := &http.Client{Timeout: 600 * time.Millisecond}
	for _, base := range candidates {
		resp, err := client.Get(base + "/api/health")
		if err != nil {
			continue
		}
		var h struct {
			OK        bool   `json:"ok"`
			VaultRoot string `json:"vault_root"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&h)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK && h.OK && canonicalPath(h.VaultRoot) == wantVault {
			return base
		}
	}
	return ""
}

func ensureDesktopDaemon() (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home dir: %w", err)
	}
	envMap := map[string]string{"GORNESS_VAULT": os.Getenv("GORNESS_VAULT")}
	vaultRoot := ResolveVaultRoot(envMap, homeDir)
	_ = os.Setenv("PATH", EnrichDesktopPATH(os.Getenv("PATH"), homeDir))

	if existing := probeHealthyDaemon(vaultRoot); existing != "" {
		return existing, vaultRoot, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", vaultRoot, err
	}
	if resolvedExe, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolvedExe
	}
	daemonBin := BundledDaemonBinary(exe)
	if _, stErr := os.Stat(daemonBin); stErr != nil {
		if pathBin, lookErr := exec.LookPath("gorness"); lookErr == nil {
			daemonBin = pathBin
		} else {
			return "", vaultRoot, fmt.Errorf("bundled engine not found at %s", daemonBin)
		}
	}

	paths := config.VaultPaths{Root: vaultRoot}
	if err := os.MkdirAll(paths.LogsDir(), 0o700); err != nil {
		return "", vaultRoot, err
	}
	logFile, err := os.OpenFile(filepath.Join(paths.LogsDir(), "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", vaultRoot, err
	}

	cmd := exec.Command(daemonBin, "daemon", "serve", "--vault", vaultRoot)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return "", vaultRoot, fmt.Errorf("start gorness daemon: %w", err)
	}
	_ = cmd.Process.Release()
	_ = logFile.Close()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if ready := probeHealthyDaemon(vaultRoot); ready != "" {
			return ready, vaultRoot, nil
		}
		time.Sleep(60 * time.Millisecond)
	}
	return "", vaultRoot, errors.New("timed out waiting for gornessd on 127.0.0.1")
}

func registerProjectDirectory(daemonBaseURL, dirPath string) error {
	payload, _ := json.Marshal(map[string]string{
		"path": dirPath,
		"name": filepath.Base(dirPath),
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(daemonBaseURL, "/")+"/api/workspaces", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("register workspace HTTP %d", resp.StatusCode)
	}
	return nil
}
