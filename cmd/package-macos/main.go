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
	"runtime"
	"strings"

	"gorness.dev/gorness/internal/config"
)

const (
	AppBundleName = "Gorness.app"
	BundleID      = "dev.gorness.desktop"
	MinMacOS      = "13.0"
)

type CmdRunner func(dir string, env []string, name string, args ...string) error

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

// NormalizeTargetArch validates --arch ("arm64", "amd64", or "universal"), defaulting to hostArch when empty,
// and returns the canonical target label and the GOARCH slice list to compile.
func NormalizeTargetArch(rawArch, hostArch string) (string, []string, error) {
	cand := strings.ToLower(strings.TrimSpace(rawArch))
	if cand == "" {
		cand = strings.ToLower(strings.TrimSpace(hostArch))
	}
	switch cand {
	case "aarch64":
		cand = "arm64"
	case "x86_64":
		cand = "amd64"
	}

	switch cand {
	case "arm64":
		return "arm64", []string{"arm64"}, nil
	case "amd64":
		return "amd64", []string{"amd64"}, nil
	case "universal":
		return "universal", []string{"arm64", "amd64"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported --arch %q (expected arm64, amd64, or universal)", rawArch)
	}
}

func clangArchForGOARCH(goarch string) string {
	if goarch == "amd64" {
		return "x86_64"
	}
	return "arm64"
}

func DMGFileName(version, targetArch string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = config.Version
	}
	arch := strings.TrimSpace(targetArch)
	if arch == "" {
		arch = runtime.GOARCH
	}
	return fmt.Sprintf("Gorness-v%s-macOS-%s.dmg", v, arch)
}

func buildSliceEnv(baseEnv []string, goarch string, enableCGO bool) []string {
	env := make([]string, 0, len(baseEnv)+5)
	env = append(env, baseEnv...)
	if !enableCGO {
		return append(env, "CGO_ENABLED=0", "GOOS=darwin", "GOARCH="+goarch)
	}
	cArch := clangArchForGOARCH(goarch)
	cFlags := fmt.Sprintf("-arch %s -mmacosx-version-min=%s", cArch, MinMacOS)
	return append(env,
		"CGO_ENABLED=1",
		"GOOS=darwin",
		"GOARCH="+goarch,
		"CGO_CFLAGS="+cFlags,
		"CGO_LDFLAGS="+cFlags,
	)
}

// BuildBinaryForTarget builds pkgPath for either a single GOARCH slice or assembles a universal fat binary via lipo -create.
func BuildBinaryForTarget(
	runner CmdRunner,
	baseEnv []string,
	repoRoot string,
	tmpDir string,
	outPath string,
	pkgPath string,
	slices []string,
	enableCGO bool,
) error {
	if len(slices) == 0 {
		return fmt.Errorf("no target architecture slices provided")
	}
	if len(slices) == 1 {
		env := buildSliceEnv(baseEnv, slices[0], enableCGO)
		return runner(repoRoot, env, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", outPath, pkgPath)
	}

	sliceOutputs := make([]string, 0, len(slices))
	baseName := filepath.Base(outPath)
	for _, goarch := range slices {
		sliceOut := filepath.Join(tmpDir, fmt.Sprintf("%s-%s", baseName, goarch))
		env := buildSliceEnv(baseEnv, goarch, enableCGO)
		if err := runner(repoRoot, env, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", sliceOut, pkgPath); err != nil {
			return fmt.Errorf("build %s (%s): %w", baseName, goarch, err)
		}
		sliceOutputs = append(sliceOutputs, sliceOut)
	}

	lipoArgs := make([]string, 0, 3+len(sliceOutputs))
	lipoArgs = append(lipoArgs, "-create", "-output", outPath)
	lipoArgs = append(lipoArgs, sliceOutputs...)
	if err := runner(repoRoot, nil, "/usr/bin/lipo", lipoArgs...); err != nil {
		return fmt.Errorf("lipo universal %s: %w", baseName, err)
	}
	for _, p := range sliceOutputs {
		_ = os.Remove(p)
	}
	return nil
}

func RenderInfoPlist(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = config.Version
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

// GenerateIconset renders the Apple Squircle Retina iconset in pure Go.
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
	const halfExtent = 0.825
	ax := math.Abs(u) / halfExtent
	ay := math.Abs(v) / halfExtent
	squircle := math.Pow(ax, 4.6) + math.Pow(ay, 4.6)
	if squircle > 1.0 {
		return 0, 0, 0, 0
	}

	t := (v + halfExtent) / (2 * halfExtent)
	r = 0.085 - 0.035*t
	g = 0.090 - 0.035*t
	b = 0.115 - 0.035*t

	if squircle > 0.945 {
		rim := (squircle - 0.945) / 0.055
		shine := 0.20 * (1.0 - 0.65*t) * rim
		r += shine
		g += shine
		b += shine * 1.15
	}

	dist := math.Hypot(u, v)
	angle := math.Atan2(v, u)

	inRing := dist >= 0.31 && dist <= 0.46
	inGap := angle > -0.64 && angle < -0.04
	inBar := u >= 0.02 && u <= 0.46 && v >= -0.068 && v <= 0.072

	if (inRing && !inGap) || inBar {
		r, g, b = 0.96, 0.97, 0.99
	}

	if dist <= 0.125 {
		r, g, b = 0.16, 0.59, 1.00
	}

	nodeDist := math.Hypot(u-0.33, v+0.25)
	if nodeDist <= 0.078 {
		r, g, b = 0.19, 0.82, 0.35
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
		archFlag    string
		installApps bool
		skipDMG     bool
	)
	flag.StringVar(&outDirFlag, "out", "dist", "Output directory for Gorness.app and .dmg")
	flag.StringVar(&archFlag, "arch", runtime.GOARCH, "Target macOS architecture (arm64, amd64, or universal)")
	flag.BoolVar(&installApps, "install", false, "Also install Gorness.app into /Applications")
	flag.BoolVar(&skipDMG, "skip-dmg", false, "Skip building .dmg disk image")
	flag.Parse()

	targetArch, archSlices, err := NormalizeTargetArch(archFlag, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}

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

	baseEnv := os.Environ()
	fmt.Printf("[2/6] Building pure-static Go engine (Contents/MacOS/gorness-engine, arch=%s)...\n", targetArch)
	if err := BuildBinaryForTarget(
		runCmd,
		baseEnv,
		repoRoot,
		tmpDir,
		filepath.Join(macosDir, "gorness-engine"),
		"./cmd/gorness",
		archSlices,
		false,
	); err != nil {
		fmt.Fprintf(os.Stderr, "build gorness engine failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[3/6] Building native macOS Cocoa/WKWebView host (Contents/MacOS/Gorness, arch=%s)...\n", targetArch)
	if err := BuildBinaryForTarget(
		runCmd,
		baseEnv,
		repoRoot,
		tmpDir,
		filepath.Join(macosDir, "Gorness"),
		"./cmd/gorness-app",
		archSlices,
		true,
	); err != nil {
		fmt.Fprintf(os.Stderr, "build Gorness native host failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[4/6] Writing Info.plist (v%s) & signing bundle...\n", config.Version)
	if err := os.WriteFile(filepath.Join(contentsDir, "Info.plist"), []byte(RenderInfoPlist(config.Version)), 0o644); err != nil {
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
		dmgPath := filepath.Join(outDir, DMGFileName(config.Version, targetArch))
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
