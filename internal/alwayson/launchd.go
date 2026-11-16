package alwayson

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorness.dev/gorness/internal/config"
)

const LaunchdLabel = "dev.gorness.daemon"

// GenerateLaunchdPlist 生成 macOS 用户级 LaunchAgent (`~/Library/LaunchAgents/dev.gorness.daemon.plist`) 的标准 XML 内容与目标路径。
func GenerateLaunchdPlist(binPath, vaultRoot string) ([]byte, string) {
	cleanBin := strings.TrimSpace(binPath)
	if cleanBin == "" {
		cleanBin = "/usr/local/bin/gorness"
	}

	cleanVault := strings.TrimSpace(vaultRoot)
	var homeDir string
	if cleanVault != "" {
		cleanVault = filepath.Clean(cleanVault)
		if filepath.Base(cleanVault) == ".gorness" {
			homeDir = filepath.Dir(cleanVault)
		}
	}
	if homeDir == "" {
		if h, err := os.UserHomeDir(); err == nil && h != "" {
			homeDir = h
		} else {
			homeDir = "/tmp"
		}
	}
	if cleanVault == "" {
		cleanVault = filepath.Join(homeDir, ".gorness")
	}

	paths := config.VaultPaths{Root: cleanVault}
	daemonLog := filepath.Join(paths.LogsDir(), "daemon.log")
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", LaunchdLabel+".plist")

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString("<dict>\n")
	fmt.Fprintf(&b, "\t<key>Label</key>\n\t<string>%s</string>\n", escapePlistXML(LaunchdLabel))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	fmt.Fprintf(&b, "\t\t<string>%s</string>\n", escapePlistXML(cleanBin))
	b.WriteString("\t\t<string>daemon</string>\n")
	b.WriteString("\t\t<string>serve</string>\n")
	b.WriteString("\t\t<string>--vault</string>\n")
	fmt.Fprintf(&b, "\t\t<string>%s</string>\n", escapePlistXML(cleanVault))
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<true/>\n")
	b.WriteString("\t<key>ProcessType</key>\n\t<string>Background</string>\n")
	fmt.Fprintf(&b, "\t<key>StandardOutPath</key>\n\t<string>%s</string>\n", escapePlistXML(daemonLog))
	fmt.Fprintf(&b, "\t<key>StandardErrorPath</key>\n\t<string>%s</string>\n", escapePlistXML(daemonLog))
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")

	return []byte(b.String()), plistPath
}

func (s *Service) GenerateLaunchdPlist(binPath, vaultRoot string) ([]byte, string) {
	if strings.TrimSpace(vaultRoot) == "" && s != nil {
		vaultRoot = s.paths.Root
	}
	return GenerateLaunchdPlist(binPath, vaultRoot)
}

func escapePlistXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
