package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/daemonctl"
)

func ResolveVaultRoot(env map[string]string, homeDir string) string {
	return daemonctl.ResolveVaultRoot("", env, homeDir)
}

// EnrichDesktopPATH prepends standard macOS developer toolchain directories without duplicates
// so Finder/Dock-launched Gorness.app sandboxes can locate git, go, node, and rg.
func EnrichDesktopPATH(currentPATH, homeDir string) string {
	return daemonctl.EnrichDesktopPATH(currentPATH, homeDir)
}

func BundledDaemonBinary(appExecutablePath string) string {
	return daemonctl.BundledDaemonBinary(appExecutablePath)
}

// IsAllowedLoopbackNavigation restricts WKWebView navigation strictly to the local daemon's origin;
// external http/https/mailto links are handed off to NSWorkspace, and cross-port loopback or file:// URLs are blocked.
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

func ensureDesktopDaemon() (*daemonctl.Endpoint, string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", "", fmt.Errorf("resolve home dir: %w", err)
	}
	envMap := map[string]string{"GORNESS_VAULT": os.Getenv("GORNESS_VAULT")}
	vaultRoot := ResolveVaultRoot(envMap, homeDir)

	var daemonBin string
	if exe, exeErr := os.Executable(); exeErr == nil && exe != "" {
		if resolvedExe, symErr := filepath.EvalSymlinks(exe); symErr == nil {
			exe = resolvedExe
		}
		daemonBin = BundledDaemonBinary(exe)
	}

	ep, err := daemonctl.EnsureDaemon(context.Background(), daemonctl.EnsureOptions{
		VaultRoot:    vaultRoot,
		DaemonBinary: daemonBin,
		HomeDir:      homeDir,
		EnrichPATH:   true,
		WaitTimeout:  8 * time.Second,
	})
	if err != nil {
		return nil, "", vaultRoot, err
	}
	webURL := ep.WebURL
	if webURL == "" {
		webURL = ep.EffectiveBaseURL()
	}
	return ep, webURL, vaultRoot, nil
}

func registerProjectDirectory(ctx context.Context, ep *daemonctl.Endpoint, fallbackBaseURL, dirPath string) (string, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	cleanDir := strings.TrimSpace(dirPath)
	name := filepath.Base(cleanDir)
	if ep != nil {
		_, raw, err := daemonctl.RegisterWorkspace(ctx, ep, cleanDir, name)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	client, baseURL := daemonctl.ClientForURL(fallbackBaseURL, "", 5*time.Second)
	_, raw, err := daemonctl.RegisterWorkspaceURL(ctx, client, baseURL, cleanDir, name)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// FormatWorkspaceJSInvocation builds a safe JavaScript snippet for WKWebView.evaluateJavaScript:
// that passes the registered workspace JSON into window.onNativeWorkspaceRegistered(...) and returns
// true when handled (avoiding Promise return type errors in WebKit), or false if undefined so Cocoa
// falls back to reloading the webview.
func FormatWorkspaceJSInvocation(rawWorkspaceJSON string) string {
	trimmed := strings.TrimSpace(rawWorkspaceJSON)
	if trimmed == "" {
		return "false"
	}
	quotedJSONLiteral, err := json.Marshal(trimmed)
	if err != nil {
		return "false"
	}
	return fmt.Sprintf(
		`(function(){if(typeof window.onNativeWorkspaceRegistered==="function"){void window.onNativeWorkspaceRegistered(%s);return true;}return false;})()`,
		string(quotedJSONLiteral),
	)
}

// PollAttentionCount queries GET /api/approvals and GET /api/alwayson via daemonctl (preferring UDS)
// and returns the combined count of live pending approvals plus pending deferred items.
func PollAttentionCount(ctx context.Context, ep *daemonctl.Endpoint, fallbackBaseURL string) (int, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
	}

	doGet := func(path string, dst any) error {
		if ep != nil {
			_, err := ep.DoJSON(ctx, http.MethodGet, path, nil, dst)
			return err
		}
		client, baseURL := daemonctl.ClientForURL(fallbackBaseURL, "", 1500*time.Millisecond)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(dst)
	}

	var approvals []core.PendingApproval
	apprErr := doGet("/api/approvals", &approvals)

	var aoState alwayson.AlwaysOnState
	aoErr := doGet("/api/alwayson", &aoState)

	if apprErr != nil && aoErr != nil {
		return 0, apprErr
	}

	deferredCount := aoState.DeferredCount
	if deferredCount == 0 && len(aoState.Deferred) > 0 {
		for _, d := range aoState.Deferred {
			st := strings.ToLower(strings.TrimSpace(d.Status))
			if st == "" || st == "pending" {
				deferredCount++
			}
		}
	}

	total := len(approvals) + deferredCount
	if total < 0 {
		total = 0
	}
	return total, nil
}

// FormatStatusBarTitle formats the macOS NSStatusItem button title with a live attention badge.
func FormatStatusBarTitle(attentionCount int) string {
	if attentionCount <= 0 {
		return "G·"
	}
	return fmt.Sprintf("G·%d", attentionCount)
}
