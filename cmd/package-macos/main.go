package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	AppBundleName = "Gorness.app"
	BundleID      = "dev.gorness.desktop"
	AppVersion    = "1.0.0"
	MinMacOS      = "13.0"
)

var reDevID = regexp.MustCompile(`"(Developer ID Application:[^"]+)"`)

func SelectSigningIdentity(envOverride string, securityOutput string) string {
	if trimmed := strings.TrimSpace(envOverride); trimmed != "" {
		return trimmed
	}
	matches := reDevID.FindAllStringSubmatch(securityOutput, -1)
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			unique = append(unique, m[1])
		}
	}
	if len(unique) == 1 {
		return unique[0]
	}
	return "-"
}

func RenderInfoPlist(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = AppVersion
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>zh_CN</string>
	<key>CFBundleDisplayName</key>
	<string>Gorness</string>
	<key>CFBundleExecutable</key>
	<string>Gorness</string>
	<key>CFBundleIconFile</key>
	<string>AppIcon</string>
	<key>CFBundleIconName</key>
	<string>AppIcon</string>
	<key>CFBundleIdentifier</key>
	<string>%s</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleLocalizations</key>
	<array>
		<string>zh_CN</string>
		<string>en</string>
	</array>
	<key>CFBundleName</key>
	<string>Gorness</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>%s</string>
	<key>CFBundleVersion</key>
	<string>%s</string>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.developer-tools</string>
	<key>LSMinimumSystemVersion</key>
	<string>%s</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key>
	<true/>
</dict>
</plist>
`, BundleID, v, v, MinMacOS)
}

// GenerateIconset 使用纯 Go 4x 超采样渲染 Apple Squircle 质感图标并生成全尺寸 Retina iconset。
func GenerateIconset(iconsetDir string) error {
	if err := os.MkdirAll(iconsetDir, 0o755); err != nil {
		return err
	}

	master := renderMasterIcon(1024)
	specs := []struct {
		name string
		size int
	}{
		{"icon_16x16.png", 16},
		{"icon_16x16@2x.png", 32},
		{"icon_32x32.png", 32},
		{"icon_32x32@2x.png", 64},
		{"icon_128x128.png", 128},
		{"icon_128x128@2x.png", 256},
		{"icon_256x256.png", 256},
		{"icon_256x256@2x.png", 512},
		{"icon_512x512.png", 512},
		{"icon_512x512@2x.png", 1024},
	}

	for _, sp := range specs {
		var img image.Image = master
		if sp.size != 1024 {
			img = downsampleBox(master, sp.size)
		}
		f, err := os.Create(filepath.Join(iconsetDir, sp.name))
		if err != nil {
			return err
		}
		encErr := png.Encode(f, img)
		closeErr := f.Close()
		if encErr != nil {
			return encErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func renderMasterIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const samples = 2
	invSamples := 1.0 / float64(samples*samples)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var rSum, gSum, bSum, aSum float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					u := ((float64(x)+((float64(sx)+0.5)/samples))/float64(size))*2.0 - 1.0
					v := ((float64(y)+((float64(sy)+0.5)/samples))/float64(size))*2.0 - 1.0
					cr, cg, cb, ca := sampleIconPixel(u, v)
					rSum += cr
					gSum += cg
					bSum += cb
					aSum += ca
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Round(rSum * invSamples * 255)),
				G: uint8(math.Round(gSum * invSamples * 255)),
				B: uint8(math.Round(bSum * invSamples * 255)),
				A: uint8(math.Round(aSum * invSamples * 255)),
			})
		}
	}
	return img
}

func sampleIconPixel(u, v float64) (r, g, b, a float64) {
	// Apple macOS Big Sur+ squircle inset (~82% canvas) with superellipse n=4.5
	const halfExtent = 0.825
	ax := math.Abs(u) / halfExtent
	ay := math.Abs(v) / halfExtent
	squircle := math.Pow(ax, 4.6) + math.Pow(ay, 4.6)
	if squircle > 1.0 {
		return 0, 0, 0, 0
	}

	// Deep Apple Space-Black anodized body with subtle vertical sheen
	t := (v + halfExtent) / (2 * halfExtent)
	r = 0.085 - 0.035*t
	g = 0.090 - 0.035*t
	b = 0.115 - 0.035*t

	// Subtle crisp inner rim highlight
	if squircle > 0.945 {
		rim := (squircle - 0.945) / 0.055
		shine := 0.20 * (1.0 - 0.65*t) * rim
		r += shine
		g += shine
		b += shine * 1.15
	}

	// Precision geometric "G" + Microkernel Core Ring + Active Cyan Node
	dist := math.Hypot(u, v)
	angle := math.Atan2(v, u) // [-pi, pi], 0 is right (+x), +pi/2 is down (+y)

	// Outer precision C-arc of the "G" (gap on upper-right [-0.62, -0.06] rad)
	inRing := dist >= 0.31 && dist <= 0.46
	inGap := angle > -0.64 && angle < -0.04
	// Horizontal crossbar of "G" from center-right (u in [0.02, 0.46], v in [-0.065, 0.075])
	inBar := u >= 0.02 && u <= 0.46 && v >= -0.068 && v <= 0.072

	if (inRing && !inGap) || inBar {
		// Clean titanium white glyph
		r, g, b = 0.96, 0.97, 0.99
	}

	// Central Cupertino Blue / Cyan microkernel pulse core
	if dist <= 0.125 {
		r, g, b = 0.16, 0.59, 1.00 // #2997FF
	}

	// Orbiting multi-agent node in the G's upper-right aperture
	nodeDist := math.Hypot(u-0.33, v+0.25)
	if nodeDist <= 0.078 {
		r, g, b = 0.19, 0.82, 0.35 // #30D158 Apple vibrant green node
	}

	return clamp01(r), clamp01(g), clamp01(b), 1.0
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func downsampleBox(src *image.NRGBA, dstSize int) *image.NRGBA {
	srcSize := src.Bounds().Dx()
	scale := srcSize / dstSize
	dst := image.NewNRGBA(image.Rect(0, 0, dstSize, dstSize))
	inv := 1.0 / float64(scale*scale)

	for dy := 0; dy < dstSize; dy++ {
		for dx := 0; dx < dstSize; dx++ {
			var r, g, b, a float64
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					c := src.NRGBAAt(dx*scale+sx, dy*scale+sy)
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a += float64(c.A)
				}
			}
			dst.SetNRGBA(dx, dy, color.NRGBA{
				R: uint8(math.Round(r * inv)),
				G: uint8(math.Round(g * inv)),
				B: uint8(math.Round(b * inv)),
				A: uint8(math.Round(a * inv)),
			})
		}
	}
	return dst
}

func runCmd(dir string, env []string, name string, args ...string) error {
	fmt.Printf("  ▶ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func detectSignIdentity() string {
	out, err := exec.Command("/usr/bin/security", "find-identity", "-v", "-p", "codesigning").CombinedOutput()
	if err != nil {
		return SelectSigningIdentity(os.Getenv("GORNESS_SIGN_IDENTITY"), "")
	}
	return SelectSigningIdentity(os.Getenv("GORNESS_SIGN_IDENTITY"), string(out))
}

func signTarget(identity, identifier, targetPath string) error {
	args := []string{"--force", "--sign", identity}
	if identifier != "" {
		args = append(args, "--identifier", identifier)
	}
	if identity != "-" {
		args = append(args, "--options", "runtime", "--timestamp=none")
	}
	args = append(args, targetPath)
	return runCmd("", nil, "/usr/bin/codesign", args...)
}

func main() {
	var (
		outDirFlag  string
		installApps bool
		skipDMG     bool
	)
	flag.StringVar(&outDirFlag, "out", "dist", "Output directory for Gorness.app and .dmg")
	flag.BoolVar(&installApps, "install", false, "Also install Gorness.app into /Applications")
	flag.BoolVar(&skipDMG, "skip-dmg", false, "Skip building .dmg disk image")
	flag.Parse()

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}
	outDir, err := filepath.Abs(outDirFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve out dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir out: %v\n", err)
		os.Exit(1)
	}

	tmpDir, err := os.MkdirTemp("", "gorness-pkg-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mktemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	stagedApp := filepath.Join(tmpDir, AppBundleName)
	contentsDir := filepath.Join(stagedApp, "Contents")
	macosDir := filepath.Join(contentsDir, "MacOS")
	resDir := filepath.Join(contentsDir, "Resources")
	for _, d := range []string{macosDir, resDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", d, err)
			os.Exit(1)
		}
	}

	fmt.Println("[1/6] Generating Retina AppIcon.icns...")
	iconsetDir := filepath.Join(tmpDir, "AppIcon.iconset")
	if err := GenerateIconset(iconsetDir); err != nil {
		fmt.Fprintf(os.Stderr, "generate iconset: %v\n", err)
		os.Exit(1)
	}
	if err := runCmd(repoRoot, nil, "/usr/bin/iconutil", "-c", "icns", iconsetDir, "-o", filepath.Join(resDir, "AppIcon.icns")); err != nil {
		fmt.Fprintf(os.Stderr, "iconutil failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[2/6] Building pure-static Go engine (Contents/MacOS/gorness-engine)...")
	noCGOEnv := append(os.Environ(), "CGO_ENABLED=0", "GOOS=darwin", "GOARCH=arm64")
	if err := runCmd(repoRoot, noCGOEnv, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(macosDir, "gorness-engine"), "./cmd/gorness"); err != nil {
		fmt.Fprintf(os.Stderr, "build gorness engine failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[3/6] Building native macOS Cocoa/WKWebView host (Contents/MacOS/Gorness)...")
	cgoEnv := append(os.Environ(), "CGO_ENABLED=1", "GOOS=darwin", "GOARCH=arm64", "CGO_CFLAGS=-mmacosx-version-min="+MinMacOS, "CGO_LDFLAGS=-mmacosx-version-min="+MinMacOS)
	if err := runCmd(repoRoot, cgoEnv, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(macosDir, "Gorness"), "./cmd/gorness-app"); err != nil {
		fmt.Fprintf(os.Stderr, "build Gorness native host failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[4/6] Writing Info.plist & signing bundle...")
	if err := os.WriteFile(filepath.Join(contentsDir, "Info.plist"), []byte(RenderInfoPlist(AppVersion)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write Info.plist: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(contentsDir, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write PkgInfo: %v\n", err)
		os.Exit(1)
	}

	identity := detectSignIdentity()
	fmt.Printf("  • Signing identity: %s\n", identity)
	_ = runCmd(repoRoot, nil, "/usr/bin/xattr", "-cr", stagedApp)
	if err := signTarget(identity, "dev.gorness.engine", filepath.Join(macosDir, "gorness-engine")); err != nil {
		fmt.Fprintf(os.Stderr, "codesign gorness-engine failed: %v\n", err)
		os.Exit(1)
	}
	if err := signTarget(identity, "", stagedApp); err != nil {
		fmt.Fprintf(os.Stderr, "codesign Gorness.app failed: %v\n", err)
		os.Exit(1)
	}
	if err := runCmd(repoRoot, nil, "/usr/bin/codesign", "--verify", "--deep", "--strict", stagedApp); err != nil {
		fmt.Fprintf(os.Stderr, "verify signature failed: %v\n", err)
		os.Exit(1)
	}

	if !skipDMG {
		fmt.Println("[5/6] Building drag-and-drop macOS Disk Image (.dmg) from clean signed bundle...")
		dmgRoot := filepath.Join(tmpDir, "dmg-root")
		_ = os.MkdirAll(dmgRoot, 0o755)
		if err := runCmd(repoRoot, nil, "/bin/cp", "-R", stagedApp, filepath.Join(dmgRoot, AppBundleName)); err != nil {
			fmt.Fprintf(os.Stderr, "stage dmg app: %v\n", err)
			os.Exit(1)
		}
		_ = os.Symlink("/Applications", filepath.Join(dmgRoot, "Applications"))
		dmgPath := filepath.Join(outDir, fmt.Sprintf("Gorness-v%s-macOS-arm64.dmg", AppVersion))
		_ = os.Remove(dmgPath)
		if err := runCmd(repoRoot, nil, "/usr/bin/hdiutil", "create", "-volname", "Gorness", "-srcfolder", dmgRoot, "-ov", "-format", "UDZO", dmgPath); err != nil {
			fmt.Fprintf(os.Stderr, "hdiutil create failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ DMG ready: %s\n", dmgPath)
	}

	fmt.Println("[6/6] Exporting dist/Gorness.app...")
	finalApp := filepath.Join(outDir, AppBundleName)
	_ = os.RemoveAll(finalApp)
	if err := runCmd(repoRoot, nil, "/bin/cp", "-R", stagedApp, finalApp); err != nil {
		fmt.Fprintf(os.Stderr, "copy final app failed: %v\n", err)
		os.Exit(1)
	}
	_ = exec.Command("/usr/bin/xattr", "-d", "com.apple.FinderInfo", finalApp).Run()

	if installApps {
		destApp := filepath.Join("/Applications", AppBundleName)
		fmt.Printf("[Install] Installing %s from clean signed bundle...\n", destApp)
		_ = os.RemoveAll(destApp)
		if err := runCmd(repoRoot, nil, "/bin/cp", "-R", stagedApp, destApp); err != nil {
			fmt.Fprintf(os.Stderr, "install /Applications/Gorness.app failed: %v\n", err)
			os.Exit(1)
		}
		if err := runCmd(repoRoot, nil, "/usr/bin/codesign", "--verify", "--deep", "--strict", destApp); err != nil {
			fmt.Fprintf(os.Stderr, "verify /Applications/Gorness.app signature failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ Installed to %s\n", destApp)
	}

	fmt.Printf("\nDone! App bundle: %s\n", finalApp)
}
