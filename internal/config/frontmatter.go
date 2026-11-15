package config

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ParseFrontmatterMD 解析支持 +++ (TOML) 与 --- (TOML 或 key: value) 头部元数据的 Markdown 文档。
func ParseFrontmatterMD(raw []byte) (meta map[string]any, body string, err error) {
	cleaned := bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	text := strings.ReplaceAll(string(cleaned), "\r\n", "\n")

	var delim string
	switch {
	case text == "+++" || strings.HasPrefix(text, "+++\n"):
		delim = "+++"
	case text == "---" || strings.HasPrefix(text, "---\n"):
		delim = "---"
	default:
		return map[string]any{}, text, nil
	}

	rest := strings.TrimPrefix(text, delim)
	rest = strings.TrimPrefix(rest, "\n")

	lines := strings.Split(rest, "\n")
	closeIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == delim {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return nil, "", fmt.Errorf("unclosed %s frontmatter header", delim)
	}

	header := strings.Join(lines[:closeIdx], "\n")
	if closeIdx+1 < len(lines) {
		body = strings.Join(lines[closeIdx+1:], "\n")
	}

	meta = make(map[string]any)
	if strings.TrimSpace(header) == "" {
		return meta, body, nil
	}

	if delim == "+++" {
		if err := toml.Unmarshal([]byte(header), &meta); err != nil {
			return nil, "", fmt.Errorf("parse +++ TOML frontmatter: %w", err)
		}
		return meta, body, nil
	}

	// --- 头优先尝试标准 TOML，若不匹配则解析兼容 SKILL.md / Agent MD 的 key: value 语法。
	if err := toml.Unmarshal([]byte(header), &meta); err == nil {
		return meta, body, nil
	}
	kvMeta, kvErr := parseKeyValueHeader(header)
	if kvErr != nil {
		return nil, "", fmt.Errorf("parse --- frontmatter: %w", kvErr)
	}
	return kvMeta, body, nil
}

func parseKeyValueHeader(header string) (map[string]any, error) {
	out := make(map[string]any)
	lines := strings.Split(header, "\n")
	var activeListKey string

	for lineNum, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") && activeListKey != "" {
			itemVal, err := parseScalarValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
			}
			existing, _ := out[activeListKey].([]any)
			out[activeListKey] = append(existing, itemVal)
			continue
		}
		activeListKey = ""

		sepIdx := strings.IndexAny(trimmed, ":=")
		if sepIdx <= 0 {
			return nil, fmt.Errorf("line %d: expected key: value or key = value", lineNum+1)
		}
		key := strings.TrimSpace(trimmed[:sepIdx])
		if !isValidFrontmatterKey(key) {
			return nil, fmt.Errorf("line %d: invalid key %q", lineNum+1, key)
		}

		valRaw := strings.TrimSpace(trimmed[sepIdx+1:])
		if valRaw == "" && trimmed[sepIdx] == ':' {
			activeListKey = key
			out[key] = []any{}
			continue
		}
		if valRaw == "" {
			return nil, fmt.Errorf("line %d: empty value for key %q", lineNum+1, key)
		}

		val, err := parseFrontmatterValue(valRaw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
		}
		out[key] = val
	}
	return out, nil
}

func isValidFrontmatterKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func parseFrontmatterValue(raw string) (any, error) {
	if strings.HasPrefix(raw, "[") {
		if !strings.HasSuffix(raw, "]") {
			return nil, errors.New("unclosed array value")
		}
		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		if inner == "" {
			return []any{}, nil
		}
		parts := splitCommaRespectingQuotes(inner)
		arr := make([]any, 0, len(parts))
		for _, p := range parts {
			v, err := parseScalarValue(strings.TrimSpace(p))
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	}
	return parseScalarValue(raw)
}

func parseScalarValue(raw string) (any, error) {
	if len(raw) >= 2 && raw[0] == '"' {
		if raw[len(raw)-1] != '"' {
			return nil, errors.New("unterminated double-quoted string")
		}
		unq, err := strconv.Unquote(raw)
		if err != nil {
			return nil, err
		}
		return unq, nil
	}
	if strings.HasPrefix(raw, "\"") {
		return nil, errors.New("unterminated double-quoted string")
	}
	if len(raw) >= 2 && raw[0] == '\'' {
		if raw[len(raw)-1] != '\'' {
			return nil, errors.New("unterminated single-quoted string")
		}
		return raw[1 : len(raw)-1], nil
	}
	if strings.HasPrefix(raw, "'") {
		return nil, errors.New("unterminated single-quoted string")
	}

	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}
	if iv, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return iv, nil
	}
	if fv, err := strconv.ParseFloat(raw, 64); err == nil && strings.ContainsAny(raw, ".eE") {
		return fv, nil
	}
	return raw, nil
}

func splitCommaRespectingQuotes(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote == '"' {
			cur.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			cur.WriteRune(r)
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			cur.WriteRune(r)
			continue
		}
		if r == ',' {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	out = append(out, cur.String())
	return out
}
