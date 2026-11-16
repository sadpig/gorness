package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestRenderInfoPlistContainsRequiredBundleMetadata(t *testing.T) {
	plist := RenderInfoPlist("1.0.0")
	for _, needle := range []string{
		"<string>Gorness</string>",
		"<string>dev.gorness.desktop</string>",
		"<string>AppIcon</string>",
		"<string>1.0.0</string>",
		"<key>NSHighResolutionCapable</key>",
		"<string>13.0</string>",
	} {
		if !strings.Contains(plist, needle) {
			t.Fatalf("RenderInfoPlist missing %q", needle)
		}
	}
}
