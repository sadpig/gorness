package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	reBearerToken = regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	reKnownKeys   = regexp.MustCompile(`\b(?:sk-ant-[A-Za-z0-9_-]{8,}|sk-[A-Za-z0-9_-]{12,}|AIza[0-9A-Za-z_-]{20,}|(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{16,})`)
	reKVSecret    = regexp.MustCompile(`(?i)\b(api[_-]?key|x-api-key|access[_-]?token|secret[_-]?key)(\s*[=:]\s*["']?)([A-Za-z0-9._~+/=-]{8,})(["']?)`)
)

// Redact 抹除显式传入的密钥字符串以及常见 Bearer Token / API Key 模式，防止敏感凭据写入日志或发往前端。
func Redact(text string, secrets []string) string {
	if text == "" {
		return ""
	}
	out := text

	if len(secrets) > 0 {
		uniq := make([]string, 0, len(secrets))
		seen := make(map[string]struct{}, len(secrets))
		for _, s := range secrets {
			if strings.TrimSpace(s) == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			uniq = append(uniq, s)
		}
		sort.Slice(uniq, func(i, j int) bool {
			return len(uniq[i]) > len(uniq[j])
		})
		for _, s := range uniq {
			out = strings.ReplaceAll(out, s, "[REDACTED]")
		}
	}

	out = reBearerToken.ReplaceAllString(out, "${1}[REDACTED]")
	out = reKnownKeys.ReplaceAllString(out, "[REDACTED]")
	out = reKVSecret.ReplaceAllString(out, "${1}${2}[REDACTED]${4}")
	return out
}

// EscapeBidi 将 Unicode 双向控制字符（Trojan Source 攻击向量）转义为显式的 <U+XXXX> 文本标记。
func EscapeBidi(text string) string {
	hasBidi := false
	for _, r := range text {
		if isBidiControl(r) {
			hasBidi = true
			break
		}
	}
	if !hasBidi {
		return text
	}

	var b strings.Builder
	b.Grow(len(text) + 16)
	for _, r := range text {
		if isBidiControl(r) {
			fmt.Fprintf(&b, "<U+%04X>", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isBidiControl(r rune) bool {
	switch {
	case r == 0x061C: // ARABIC LETTER MARK
		return true
	case r == 0x200E || r == 0x200F: // LRM, RLM
		return true
	case r >= 0x202A && r <= 0x202E: // LRE, RLE, PDF, LRO, RLO
		return true
	case r >= 0x2066 && r <= 0x2069: // LRI, RLI, FSI, PDI
		return true
	default:
		return false
	}
}
