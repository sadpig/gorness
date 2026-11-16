package config

import (
	"fmt"
	"path"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// HookCommandSpec defines an external shell hook command configured via hooks.toml.
type HookCommandSpec struct {
	ID         string   `toml:"id,omitempty" json:"id,omitempty"`
	Matcher    string   `toml:"matcher,omitempty" json:"matcher,omitempty"`
	Tools      []string `toml:"tools,omitempty" json:"tools,omitempty"`
	Command    string   `toml:"command" json:"command"`
	TimeoutSec int      `toml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
	SourceFile string   `toml:"-" json:"source_file,omitempty"`
}

// Clone returns a deep copy of HookCommandSpec.
func (h HookCommandSpec) Clone() HookCommandSpec {
	cp := h
	if h.Tools != nil {
		cp.Tools = make([]string, len(h.Tools))
		copy(cp.Tools, h.Tools)
	}
	return cp
}

// MatchesTool reports whether this hook applies to toolName.
// Empty Tools and empty/"*" Matcher match all tools.
func (h HookCommandSpec) MatchesTool(toolName string) bool {
	target := strings.TrimSpace(toolName)
	if len(h.Tools) > 0 {
		matchedTool := false
		for _, pat := range h.Tools {
			p := strings.TrimSpace(pat)
			if p == "" || p == "*" || strings.EqualFold(p, target) {
				matchedTool = true
				break
			}
			if ok, _ := path.Match(strings.ToLower(p), strings.ToLower(target)); ok {
				matchedTool = true
				break
			}
		}
		if !matchedTool {
			return false
		}
	}
	matcher := strings.TrimSpace(h.Matcher)
	if matcher == "" || matcher == "*" {
		return true
	}
	for _, seg := range strings.Split(matcher, "|") {
		pat := strings.TrimSpace(seg)
		if pat == "" {
			continue
		}
		if pat == "*" || strings.EqualFold(pat, target) {
			return true
		}
		if ok, _ := path.Match(strings.ToLower(pat), strings.ToLower(target)); ok {
			return true
		}
	}
	return false
}

// HooksConfig aggregates PreToolUse, PostToolUse, and TurnStopping command hooks.
type HooksConfig struct {
	PreToolUse   []HookCommandSpec `toml:"pre_tool_use,omitempty" json:"pre_tool_use,omitempty"`
	PostToolUse  []HookCommandSpec `toml:"post_tool_use,omitempty" json:"post_tool_use,omitempty"`
	TurnStopping []HookCommandSpec `toml:"turn_stopping,omitempty" json:"turn_stopping,omitempty"`
}

// IsEmpty reports whether no command hooks are configured across any lifecycle phase.
func (c HooksConfig) IsEmpty() bool {
	return len(c.PreToolUse) == 0 && len(c.PostToolUse) == 0 && len(c.TurnStopping) == 0
}

// Clone returns a deep copy of HooksConfig.
func (c HooksConfig) Clone() HooksConfig {
	var out HooksConfig
	if c.PreToolUse != nil {
		out.PreToolUse = make([]HookCommandSpec, len(c.PreToolUse))
		for i, h := range c.PreToolUse {
			out.PreToolUse[i] = h.Clone()
		}
	}
	if c.PostToolUse != nil {
		out.PostToolUse = make([]HookCommandSpec, len(c.PostToolUse))
		for i, h := range c.PostToolUse {
			out.PostToolUse[i] = h.Clone()
		}
	}
	if c.TurnStopping != nil {
		out.TurnStopping = make([]HookCommandSpec, len(c.TurnStopping))
		for i, h := range c.TurnStopping {
			out.TurnStopping[i] = h.Clone()
		}
	}
	return out
}

// Merge appends overlay command hooks after c and returns the combined HooksConfig.
func (c HooksConfig) Merge(overlay HooksConfig) HooksConfig {
	out := c.Clone()
	for _, h := range overlay.PreToolUse {
		out.PreToolUse = append(out.PreToolUse, h.Clone())
	}
	for _, h := range overlay.PostToolUse {
		out.PostToolUse = append(out.PostToolUse, h.Clone())
	}
	for _, h := range overlay.TurnStopping {
		out.TurnStopping = append(out.TurnStopping, h.Clone())
	}
	return out
}

type rawHooksBlock struct {
	PreToolUse       []HookCommandSpec `toml:"pre_tool_use"`
	PreToolUsePascal []HookCommandSpec `toml:"PreToolUse"`
	PostToolUse      []HookCommandSpec `toml:"post_tool_use"`
	PostToolPascal   []HookCommandSpec `toml:"PostToolUse"`
	TurnStopping     []HookCommandSpec `toml:"turn_stopping"`
	TurnStopPascal   []HookCommandSpec `toml:"TurnStopping"`
}

type rawHooksFile struct {
	PreToolUse       []HookCommandSpec `toml:"pre_tool_use"`
	PreToolUsePascal []HookCommandSpec `toml:"PreToolUse"`
	PostToolUse      []HookCommandSpec `toml:"post_tool_use"`
	PostToolPascal   []HookCommandSpec `toml:"PostToolUse"`
	TurnStopping     []HookCommandSpec `toml:"turn_stopping"`
	TurnStopPascal   []HookCommandSpec `toml:"TurnStopping"`
	Hooks            *rawHooksBlock    `toml:"hooks"`
}

func parseHooksTOML(raw []byte, sourcePath string) (HooksConfig, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return HooksConfig{}, nil
	}
	var doc rawHooksFile
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return HooksConfig{}, err
	}

	normalizeList := func(phase string, lists ...[]HookCommandSpec) ([]HookCommandSpec, error) {
		var out []HookCommandSpec
		for _, list := range lists {
			for idx, item := range list {
				cmd := strings.TrimSpace(item.Command)
				if cmd == "" {
					return nil, fmt.Errorf("%s: %s[%d] command must not be empty", sourcePath, phase, idx)
				}
				spec := item.Clone()
				spec.Command = cmd
				spec.ID = strings.TrimSpace(spec.ID)
				spec.Matcher = strings.TrimSpace(spec.Matcher)
				spec.SourceFile = sourcePath
				if spec.TimeoutSec <= 0 {
					spec.TimeoutSec = 10
				}
				out = append(out, spec)
			}
		}
		return out, nil
	}

	var nestedPre, nestedPreP, nestedPost, nestedPostP, nestedStop, nestedStopP []HookCommandSpec
	if doc.Hooks != nil {
		nestedPre = doc.Hooks.PreToolUse
		nestedPreP = doc.Hooks.PreToolUsePascal
		nestedPost = doc.Hooks.PostToolUse
		nestedPostP = doc.Hooks.PostToolPascal
		nestedStop = doc.Hooks.TurnStopping
		nestedStopP = doc.Hooks.TurnStopPascal
	}

	pre, err := normalizeList("PreToolUse", doc.PreToolUse, doc.PreToolUsePascal, nestedPre, nestedPreP)
	if err != nil {
		return HooksConfig{}, err
	}
	post, err := normalizeList("PostToolUse", doc.PostToolUse, doc.PostToolPascal, nestedPost, nestedPostP)
	if err != nil {
		return HooksConfig{}, err
	}
	stop, err := normalizeList("TurnStopping", doc.TurnStopping, doc.TurnStopPascal, nestedStop, nestedStopP)
	if err != nil {
		return HooksConfig{}, err
	}

	return HooksConfig{
		PreToolUse:   pre,
		PostToolUse:  post,
		TurnStopping: stop,
	}, nil
}

const defaultHooksTOML = `# Gorness Lifecycle Command Hooks (工具与轮次生命周期命令钩子)
# PreToolUse exit code 2 blocks the tool call and returns stderr feedback to the model.
# Workspace .gorness/hooks.toml requires workspace trust before execution.
#
# [[pre_tool_use]]
# id = "audit-shell"
# matcher = "run_command"
# command = "cat >/dev/null"
# timeout_sec = 10
`
