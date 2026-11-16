package main

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorness.dev/gorness/internal/config"
)

func TestGenerateIconsetProducesAllTenRetinaPNGs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "AppIcon.iconset")
	if err := GenerateIconset(dir); err != nil {
		t.Fatalf("GenerateIconset error: %v", err)
	}

	expected := map[string]int{
		"icon_16x16.png":      16,
		"icon_16x16@2x.png":   32,
		"icon_32x32.png":      32,
		"icon_32x32@2x.png":   64,
		"icon_128x128.png":    128,
		"icon_128x128@2x.png": 256,
		"icon_256x256.png":    256,
		"icon_256x256@2x.png": 512,
		"icon_512x512.png":    512,
		"icon_512x512@2x.png": 1024,
	}

	for name, wantPx := range expected {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		cfg, err := png.DecodeConfig(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("invalid png %s: %v", name, err)
		}
		if cfg.Width != wantPx || cfg.Height != wantPx {
			t.Fatalf("%s dimensions = %dx%d, want %dx%d", name, cfg.Width, cfg.Height, wantPx, wantPx)
		}
	}
}

func TestSelectSigningIdentityPicksSingleDeveloperIDOrFallsBack(t *testing.T) {
	sample := `
  1) 50A137300619A137C8D92ABEC6A18DADA5889829 "Apple Development: Wei Tan (7XA8D6V6KZ)"
  2) E67780D32FEFC6F757E3965E6BFC692E6CF6C757 "Developer ID Application: Wei Tan (DWR5AEJSYY)"
     2 valid identities found`

	if got := SelectSigningIdentity("", sample); got != "Developer ID Application: Wei Tan (DWR5AEJSYY)" {
		t.Fatalf("SelectSigningIdentity = %q", got)
	}
	if got := SelectSigningIdentity("-", sample); got != "-" {
		t.Fatalf("explicit '-' override got %q", got)
	}
	ambiguous := sample + "\n  3) AAAA \"Developer ID Application: Second Org (ZZZZ)\""
	if got := SelectSigningIdentity("", ambiguous); got != "-" {
		t.Fatalf("ambiguous Developer IDs should fall back to '-', got %q", got)
	}
}

func TestRenderInfoPlistContainsRequiredBundleMetadataAndConfigVersion(t *testing.T) {
	plist := RenderInfoPlist("")
	for _, needle := range []string{
		"<string>Gorness</string>",
		"<string>dev.gorness.desktop</string>",
		"<string>AppIcon</string>",
		"<string>" + config.Version + "</string>",
		"<key>NSHighResolutionCapable</key>",
		"<string>13.0</string>",
	} {
		if !strings.Contains(plist, needle) {
			t.Fatalf("RenderInfoPlist missing %q", needle)
		}
	}
}

func TestNormalizeTargetArchAndDMGFileName(t *testing.T) {
	arch, slices, err := NormalizeTargetArch("", "amd64")
	if err != nil || arch != "amd64" || !reflect.DeepEqual(slices, []string{"amd64"}) {
		t.Fatalf("default hostArch amd64 failed: arch=%q slices=%v err=%v", arch, slices, err)
	}

	arch, slices, err = NormalizeTargetArch("arm64", "amd64")
	if err != nil || arch != "arm64" || !reflect.DeepEqual(slices, []string{"arm64"}) {
		t.Fatalf("arm64 failed: arch=%q slices=%v err=%v", arch, slices, err)
	}

	arch, slices, err = NormalizeTargetArch("universal", "arm64")
	if err != nil || arch != "universal" || !reflect.DeepEqual(slices, []string{"arm64", "amd64"}) {
		t.Fatalf("universal failed: arch=%q slices=%v err=%v", arch, slices, err)
	}

	if _, _, err := NormalizeTargetArch("riscv64", "arm64"); err == nil {
		t.Fatal("expected unsupported arch error for riscv64")
	}

	wantDMG := "Gorness-v" + config.Version + "-macOS-universal.dmg"
	if got := DMGFileName("", "universal"); got != wantDMG {
		t.Fatalf("DMGFileName = %q, want %q", got, wantDMG)
	}
}

func TestBuildBinaryForTargetSingleAndUniversalLipo(t *testing.T) {
	type recordedCall struct {
		env  []string
		name string
		args []string
	}
	var calls []recordedCall
	runner := func(_ string, env []string, name string, args ...string) error {
		calls = append(calls, recordedCall{env: env, name: name, args: args})
		return nil
	}

	tmpDir := t.TempDir()
	outSingle := filepath.Join(tmpDir, "gorness-engine")
	if err := BuildBinaryForTarget(runner, nil, "/repo", tmpDir, outSingle, "./cmd/gorness", []string{"amd64"}, false); err != nil {
		t.Fatalf("single-arch BuildBinaryForTarget error: %v", err)
	}
	if len(calls) != 1 || calls[0].name != "go" {
		t.Fatalf("expected 1 go build call for single-arch, got %+v", calls)
	}
	envJoined := strings.Join(calls[0].env, " ")
	if !strings.Contains(envJoined, "CGO_ENABLED=0") || !strings.Contains(envJoined, "GOARCH=amd64") {
		t.Fatalf("unexpected single-arch env: %s", envJoined)
	}

	calls = nil
	outUniv := filepath.Join(tmpDir, "Gorness")
	if err := BuildBinaryForTarget(runner, nil, "/repo", tmpDir, outUniv, "./cmd/gorness-app", []string{"arm64", "amd64"}, true); err != nil {
		t.Fatalf("universal BuildBinaryForTarget error: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls (arm64 build, amd64 build, lipo -create), got %d: %+v", len(calls), calls)
	}
	if !strings.Contains(strings.Join(calls[0].env, " "), "-arch arm64") {
		t.Fatalf("expected -arch arm64 CGO flags on arm64 slice, got %v", calls[0].env)
	}
	if !strings.Contains(strings.Join(calls[1].env, " "), "-arch x86_64") {
		t.Fatalf("expected -arch x86_64 CGO flags on amd64 slice, got %v", calls[1].env)
	}
	if calls[2].name != "/usr/bin/lipo" || len(calls[2].args) != 5 || calls[2].args[0] != "-create" || calls[2].args[1] != "-output" || calls[2].args[2] != outUniv {
		t.Fatalf("unexpected lipo invocation: %s %v", calls[2].name, calls[2].args)
	}
}
