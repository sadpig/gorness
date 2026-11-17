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

func TestSequoiaIconShaderAndSupersampling(t *testing.T) {
	if IconSupersample != 4 {
		t.Fatalf("IconSupersample = %d, want 4 (4x4=16x supersampling)", IconSupersample)
	}

	// 1. Continuous curvature Squircle geometry (n=5.0, halfExtent=0.824)
	if _, _, _, a := sampleIconPixel(0.75, 0.75); a != 0 {
		t.Fatalf("rounded squircle corner (0.75, 0.75) should be transparent, got alpha=%v", a)
	}
	if _, _, _, a := sampleIconPixel(0.80, 0.0); a != 1.0 {
		t.Fatalf("axial interior (0.80, 0.0) should be opaque, got alpha=%v", a)
	}

	// 2. Cosmic Deep Indigo gradient + upper radial highlight
	topR, topG, topB, _ := sampleIconPixel(-0.08, -0.58)
	botR, botG, botB, _ := sampleIconPixel(0.08, 0.65)
	topLum := 0.2126*topR + 0.7152*topG + 0.0722*topB
	botLum := 0.2126*botR + 0.7152*botG + 0.0722*botB
	if topLum <= botLum*1.5 || topB <= botB {
		t.Fatalf("expected upper Cosmic Deep Indigo + radial highlight (%v,%v,%v) to be brighter/bluer than bottom (%v,%v,%v)",
			topR, topG, topB, botR, botG, botB)
	}

	// 3. Top crystal glass rim highlight vs inner background, and bottom ambient occlusion
	rimR, rimG, rimB, _ := sampleIconPixel(0.0, -0.8225)
	inR, inG, inB, _ := sampleIconPixel(0.0, -0.760)
	if (rimR+rimG+rimB) <= (inR+inG+inB) {
		t.Fatalf("expected top glass bevel at (0,-0.8225) to be brighter than interior (0,-0.760): rim=(%v,%v,%v) in=(%v,%v,%v)",
			rimR, rimG, rimB, inR, inG, inB)
	}
	aoR, aoG, aoB, _ := sampleIconPixel(0.0, 0.814)
	baseBotR, baseBotG, baseBotB, _ := sampleIconPixel(0.0, 0.740)
	if (aoR+aoG+aoB) >= (baseBotR+baseBotG+baseBotB) {
		t.Fatalf("expected bottom inner ambient occlusion at (0,0.814) to darken base: ao=(%v,%v,%v) base=(%v,%v,%v)",
			aoR, aoG, aoB, baseBotR, baseBotG, baseBotB)
	}

	// 4. Central 3D crystal "G", glowing core, dual 30° orbits, and Cyan/Amber/Violet agent nodes
	coreR, coreG, coreB, _ := sampleIconPixel(0.0, 0.0)
	if coreB < 0.90 || coreG < 0.50 || coreR > coreB {
		t.Fatalf("unexpected central core color: (%v, %v, %v)", coreR, coreG, coreB)
	}
	gRingR, gRingG, gRingB, _ := sampleIconPixel(-0.31, 0.0)
	if gRingR < 0.65 || gRingG < 0.80 || gRingB < 0.95 {
		t.Fatalf("expected bright crystal G ring at (-0.31, 0), got (%v, %v, %v)", gRingR, gRingG, gRingB)
	}

	// Primary (-30°) and secondary (+30°) tilted orbit luminescence
	orb1R, orb1G, orb1B, _ := sampleIconPixel(-0.12, -0.208) // lies on e1 ≈ 1.0
	bgR, bgG, bgB, _ := sampleIconPixel(-0.02, -0.50)
	if orb1B <= bgB || orb1G <= bgG {
		t.Fatalf("expected primary -30° orbit at (-0.12,-0.208) to glow above background: orb=(%v,%v,%v) bg=(%v,%v,%v)",
			orb1R, orb1G, orb1B, bgR, bgG, bgB)
	}
	orb2R, orb2G, orb2B, _ := sampleIconPixel(0.450, 0.260) // lies on +30° major axis end (e2 ≈ 1.0)
	if orb2B <= botB*2.0 || orb2R <= botR*2.0 {
		t.Fatalf("expected secondary +30° orbit at (0.450,0.260) to glow above deep background: orb2=(%v,%v,%v)",
			orb2R, orb2G, orb2B)
	}

	// Cyan node (0.48, -0.27), Amber node (-0.42, 0.34), Violet node (-0.34, -0.31)
	cR, cG, cB, _ := sampleIconPixel(0.48, -0.27)
	if cB < 0.75 || cG < 0.70 || cR > cG {
		t.Fatalf("expected Cyan agent micro-core at (0.48,-0.27), got (%v,%v,%v)", cR, cG, cB)
	}
	aR, aG, aB, _ := sampleIconPixel(-0.42, 0.34)
	if aR < 0.80 || aG < 0.50 || aB > aG {
		t.Fatalf("expected Amber agent micro-core at (-0.42,0.34), got (%v,%v,%v)", aR, aG, aB)
	}
	vR, vG, vB, _ := sampleIconPixel(-0.34, -0.31)
	if vB < 0.80 || vR < 0.55 || vG >= vR {
		t.Fatalf("expected Violet agent micro-core at (-0.34,-0.31), got (%v,%v,%v)", vR, vG, vB)
	}

	// 5. Verify 4x4 supersampling produces smooth fractional alpha values along the squircle edge
	img := renderMasterIcon(128)
	partialAlphaCount := 0
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			a := img.NRGBAAt(x, y).A
			if a > 0 && a < 255 {
				partialAlphaCount++
			}
		}
	}
	if partialAlphaCount < 100 {
		t.Fatalf("expected smooth antialiased edge pixels from 4x4 supersampling, got only %d partial-alpha pixels", partialAlphaCount)
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
