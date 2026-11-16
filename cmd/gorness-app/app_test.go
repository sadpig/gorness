package main

import (
	"path/filepath"
	"strings"
	"testing"
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
