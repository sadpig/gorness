package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorness.dev/gorness/internal/config"
)

const (
	defaultProviderTimeout = 120 * time.Second
	maxRetriesPerRoute     = 2
	timeoutScaleFactor     = 1.5
	defaultRetryBackoff    = 15 * time.Millisecond
)

type ResolvedRoute struct {
	RoleID           string
	ProviderID       string
	ModelID          string
	Effort           string
	Provider         config.ProviderConfig
	Spec             config.ModelSpec
	Fallback         *ResolvedRoute
	apiKeySourceFile string
}

// Router 负责异构 Agent 角色的四级优先级路由、受控凭据解析、1.5x 超时退避重试、中途断流步重置与跨模型平滑回退。
type Router struct {
	cfg       *config.Manager
	secrets   config.SecretResolver
	transport Streamer
	backoff   func(retryIdx int) time.Duration
}

func NewRouter(cfg *config.Manager, secrets config.SecretResolver, transport Streamer) *Router {
	if secrets == nil && cfg != nil {
		secrets = cfg.Secrets()
	}
	if transport == nil {
		transport = NewNativeStreamer(nil)
	}
	return &Router{
		cfg:       cfg,
		secrets:   secrets,
		transport: transport,
		backoff: func(retryIdx int) time.Duration {
			return time.Duration(retryIdx+1) * defaultRetryBackoff
		},
	}
}

// Resolve 按严格优先级解析目标模型与回退链：
// runModelOverride > roleProfile (agents/*.md) > wsModelOverride > globalDefault (config.toml)。
func (r *Router) Resolve(roleID, wsModelOverride, runModelOverride string, snap *config.Snapshot) (ResolvedRoute, error) {
	if snap == nil && r != nil && r.cfg != nil {
		snap = r.cfg.Current()
	}
	if snap == nil {
		return ResolvedRoute{}, errors.New("llm router: nil configuration snapshot")
	}

	cleanRole := strings.TrimSpace(roleID)
	runSel := strings.TrimSpace(runModelOverride)
	wsSel := strings.TrimSpace(wsModelOverride)

	prof, hasProf := snap.Agents[cleanRole]

	effort := strings.TrimSpace(snap.Global.DefaultEffort)
	if hasProf && strings.TrimSpace(prof.Effort) != "" {
		effort = strings.TrimSpace(prof.Effort)
	}
	if effort == "" {
		effort = "high"
	}

	var (
		primary       ResolvedRoute
		err           error
		usedRoleModel bool
	)

	switch {
	case runSel != "":
		hintProv := snap.Global.DefaultProvider
		if hasProf && strings.TrimSpace(prof.Provider) != "" {
			hintProv = strings.TrimSpace(prof.Provider)
		}
		primary, err = resolveSelectorToRoute(cleanRole, effort, runSel, hintProv, snap)

	case hasProf && strings.TrimSpace(prof.Model) != "":
		usedRoleModel = true
		primary, err = resolveProfilePrimary(cleanRole, effort, prof, snap)

	case wsSel != "":
		primary, err = resolveSelectorToRoute(cleanRole, effort, wsSel, snap.Global.DefaultProvider, snap)

	default:
		primary, err = resolveSelectorToRoute(cleanRole, effort, snap.Global.DefaultModel, snap.Global.DefaultProvider, snap)
	}
	if err != nil {
		return ResolvedRoute{}, err
	}

	// 若角色 Profile 自身显式声明了 api_key 覆盖，独立记录其来源路径以确保 key 与 provider headers 各自遵循严密 cmd: 信任边界。
	if hasProf && strings.TrimSpace(prof.APIKeyRef) != "" && (usedRoleModel || primary.ProviderID == strings.TrimSpace(prof.Provider)) {
		primary.Provider.APIKeyRef = prof.APIKeyRef
		primary.apiKeySourceFile = prof.SourceFile
	}

	// 构建去重后的回退链表（支持 "provider/model" 与裸 "model" 语法）。
	if hasProf && len(prof.Fallbacks) > 0 {
		seen := map[string]struct{}{
			primary.ProviderID + "/" + primary.ModelID: {},
		}
		tail := &primary
		for _, fbRaw := range prof.Fallbacks {
			fbSel := strings.TrimSpace(fbRaw)
			if fbSel == "" {
				continue
			}
			fbRoute, fbErr := resolveSelectorToRoute(cleanRole, effort, fbSel, snap.Global.DefaultProvider, snap)
			if fbErr != nil {
				continue
			}
			key := fbRoute.ProviderID + "/" + fbRoute.ModelID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			next := fbRoute
			tail.Fallback = &next
			tail = tail.Fallback
		}
	}

	return primary, nil
}

func resolveProfilePrimary(roleID, effort string, prof config.AgentProfile, snap *config.Snapshot) (ResolvedRoute, error) {
	provID := strings.TrimSpace(prof.Provider)
	modelStr := strings.TrimSpace(prof.Model)

	if provID != "" {
		if p, ok := snap.Providers[provID]; ok {
			// 若显式指定了 provider，仅当 modelStr 为跨厂商 "otherProv/model" 且不在当前 provider 模型表时才切分。
			if _, hasExact := p.Models[modelStr]; hasExact || !strings.Contains(modelStr, "/") {
				spec := p.Models[modelStr]
				if spec.ID == "" {
					spec.ID = modelStr
				}
				return ResolvedRoute{
					RoleID:     roleID,
					ProviderID: provID,
					ModelID:    modelStr,
					Effort:     effort,
					Provider:   cloneProviderConfig(p),
					Spec:       spec,
				}, nil
			}
		}
	}
	return resolveSelectorToRoute(roleID, effort, modelStr, provID, snap)
}

func resolveSelectorToRoute(roleID, effort, selector, preferredProvider string, snap *config.Snapshot) (ResolvedRoute, error) {
	sel := strings.TrimSpace(selector)
	pref := strings.TrimSpace(preferredProvider)
	if sel == "" {
		return ResolvedRoute{}, errors.New("llm router: empty model selector")
	}

	// 1. 优先尝试 "provider/model" 显式限定语法。
	if prefix, rest, hasSlash := strings.Cut(sel, "/"); hasSlash && prefix != "" && rest != "" {
		if p, ok := snap.Providers[prefix]; ok {
			spec := p.Models[rest]
			if spec.ID == "" {
				spec.ID = rest
			}
			return ResolvedRoute{
				RoleID:     roleID,
				ProviderID: prefix,
				ModelID:    rest,
				Effort:     effort,
				Provider:   cloneProviderConfig(p),
				Spec:       spec,
			}, nil
		}
		// 若 preferredProvider 拥有含斜杠的全名模型（例如 OpenRouter 上的 "qwen/qwen3-coder"），允许命中。
		if pref != "" {
			if p, ok := snap.Providers[pref]; ok {
				if spec, has := p.Models[sel]; has {
					if spec.ID == "" {
						spec.ID = sel
					}
					return ResolvedRoute{
						RoleID:     roleID,
						ProviderID: pref,
						ModelID:    sel,
						Effort:     effort,
						Provider:   cloneProviderConfig(p),
						Spec:       spec,
					}, nil
				}
			}
		}
		return ResolvedRoute{}, fmt.Errorf("llm router: unknown provider %q in model selector %q", prefix, sel)
	}

	// 2. 裸模型名优先匹配 preferredProvider 的已声明模型。
	if pref != "" {
		if p, ok := snap.Providers[pref]; ok {
			if spec, has := p.Models[sel]; has {
				if spec.ID == "" {
					spec.ID = sel
				}
				return ResolvedRoute{
					RoleID:     roleID,
					ProviderID: pref,
					ModelID:    sel,
					Effort:     effort,
					Provider:   cloneProviderConfig(p),
					Spec:       spec,
				}, nil
			}
		}
	}

	// 3. 匹配全局 DefaultProvider 的已声明模型。
	defProv := strings.TrimSpace(snap.Global.DefaultProvider)
	if defProv != "" {
		if p, ok := snap.Providers[defProv]; ok {
			if spec, has := p.Models[sel]; has {
				if spec.ID == "" {
					spec.ID = sel
				}
				return ResolvedRoute{
					RoleID:     roleID,
					ProviderID: defProv,
					ModelID:    sel,
					Effort:     effort,
					Provider:   cloneProviderConfig(p),
					Spec:       spec,
				}, nil
			}
		}
	}

	// 4. 确定性遍历所有已注册 Provider，定位拥有该裸模型的供应商。
	provIDs := make([]string, 0, len(snap.Providers))
	for id := range snap.Providers {
		provIDs = append(provIDs, id)
	}
	sort.Strings(provIDs)
	for _, pid := range provIDs {
		p := snap.Providers[pid]
		if spec, has := p.Models[sel]; has {
			if spec.ID == "" {
				spec.ID = sel
			}
			return ResolvedRoute{
				RoleID:     roleID,
				ProviderID: pid,
				ModelID:    sel,
				Effort:     effort,
				Provider:   cloneProviderConfig(p),
				Spec:       spec,
			}, nil
		}
	}

	// 5. 动态未登记模型（如本地新拉取的 Ollama/MLX 标签）回落至 preferredProvider 或 DefaultProvider。
	for _, fallbackProvID := range []string{pref, defProv} {
		if fallbackProvID == "" {
			continue
		}
		if p, ok := snap.Providers[fallbackProvID]; ok {
			return ResolvedRoute{
				RoleID:     roleID,
				ProviderID: fallbackProvID,
				ModelID:    sel,
				Effort:     effort,
				Provider:   cloneProviderConfig(p),
				Spec:       config.ModelSpec{ID: sel},
			}, nil
		}
	}

	return ResolvedRoute{}, fmt.Errorf("llm router: model %q not found in any configured provider", sel)
}

func (r *Router) StreamWithRetry(ctx context.Context, route ResolvedRoute, req Request) (<-chan StreamEvent, error) {
	if r == nil || r.transport == nil {
		return nil, errors.New("llm router: uninitialized transport")
	}
	if strings.TrimSpace(route.Provider.Protocol) == "" && strings.TrimSpace(route.ModelID) == "" {
		return nil, errors.New("llm router: empty route")
	}

	// 预先同步核验主路由凭据，确保未授信项目覆盖中的 cmd: 违规调用立即返回 ErrCmdSecretDisallowed。
	firstProv, firstKey, firstSecrets, credErr := r.prepareRouteCredentials(ctx, route, req.APIKey)
	if credErr != nil {
		if errors.Is(credErr, config.ErrCmdSecretDisallowed) || route.Fallback == nil {
			return nil, r.scrubWithSecrets(credErr, firstSecrets...)
		}
	}

	out := make(chan StreamEvent, 64)
	go func() {
		defer close(out)

		totalAttempt := 0
		var gatheredSecrets []string
		gatheredSecrets = append(gatheredSecrets, firstSecrets...)
		if req.APIKey != "" {
			gatheredSecrets = append(gatheredSecrets, req.APIKey)
		}

		for cur, isFirst := &route, true; cur != nil; cur, isFirst = cur.Fallback, false {
			var (
				resolvedProv config.ProviderConfig
				apiKey       string
				err          error
			)
			if isFirst {
				resolvedProv = firstProv
				apiKey = firstKey
				err = credErr
			} else {
				var moreSecrets []string
				resolvedProv, apiKey, moreSecrets, err = r.prepareRouteCredentials(ctx, *cur, "")
				gatheredSecrets = append(gatheredSecrets, moreSecrets...)
			}

			if err != nil {
				scrubbed := r.scrubWithSecrets(err, gatheredSecrets...)
				if errors.Is(err, config.ErrCmdSecretDisallowed) || cur.Fallback == nil {
					emitEvent(ctx, out, StreamEvent{
						Type:    StreamError,
						Attempt: totalAttempt + 1,
						Err:     scrubbed,
					})
					return
				}
				emitEvent(ctx, out, StreamEvent{
					Type:    StreamRetry,
					Attempt: totalAttempt + 1,
					Err:     scrubbed,
				})
				continue
			}

			baseTimeout := defaultProviderTimeout
			if resolvedProv.TimeoutSec > 0 {
				baseTimeout = time.Duration(resolvedProv.TimeoutSec) * time.Second
			}

			for retryIdx := 0; retryIdx <= maxRetriesPerRoute; retryIdx++ {
				if ctx.Err() != nil {
					emitEvent(ctx, out, StreamEvent{
						Type:    StreamError,
						Attempt: totalAttempt + 1,
						Err:     r.scrubWithSecrets(ctx.Err(), gatheredSecrets...),
					})
					return
				}

				scale := 1.0
				for i := 0; i < retryIdx; i++ {
					scale *= timeoutScaleFactor
				}
				attemptTimeout := time.Duration(float64(baseTimeout) * scale)
				totalAttempt++

				attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
				attemptReq := req
				attemptReq.Provider = resolvedProv
				attemptReq.APIKey = apiKey
				if cur.ModelID != "" {
					attemptReq.Model = cur.ModelID
				}
				if strings.TrimSpace(attemptReq.Effort) == "" {
					attemptReq.Effort = cur.Effort
				}

				emittedPartial, completed, attemptErr := r.runSingleAttempt(attemptCtx, attemptReq, totalAttempt, out)
				cancel()

				if completed {
					return
				}

				// 若父级 ctx 已被调用方主动取消/超时，立即停止，绝不发起多余重试或回退。
				if ctx.Err() != nil {
					emitEvent(ctx, out, StreamEvent{
						Type:    StreamError,
						Attempt: totalAttempt,
						Err:     r.scrubWithSecrets(ctx.Err(), gatheredSecrets...),
					})
					return
				}

				scrubbedErr := r.scrubWithSecrets(attemptErr, gatheredSecrets...)
				canRetrySameRoute := retryIdx < maxRetriesPerRoute && isRetryableError(attemptErr)

				if canRetrySameRoute {
					if emittedPartial {
						if !emitEvent(ctx, out, StreamEvent{
							Type:    StreamStepReset,
							Attempt: totalAttempt,
							Err:     scrubbedErr,
						}) {
							return
						}
					}
					if !emitEvent(ctx, out, StreamEvent{
						Type:    StreamRetry,
						Attempt: totalAttempt,
						Err:     scrubbedErr,
					}) {
						return
					}
					if !r.sleepBackoff(ctx, retryIdx) {
						return
					}
					continue
				}

				// 当前路由重试耗尽或遇不可原地重试错误：若存在备选回退路由，重置半截状态并切换至 Fallback。
				if cur.Fallback != nil {
					if emittedPartial {
						if !emitEvent(ctx, out, StreamEvent{
							Type:    StreamStepReset,
							Attempt: totalAttempt,
							Err:     scrubbedErr,
						}) {
							return
						}
					}
					if !emitEvent(ctx, out, StreamEvent{
						Type:    StreamRetry,
						Attempt: totalAttempt,
						Err:     scrubbedErr,
					}) {
						return
					}
					break
				}

				emitEvent(ctx, out, StreamEvent{
					Type:    StreamError,
					Attempt: totalAttempt,
					Err:     scrubbedErr,
				})
				return
			}
		}
	}()

	return out, nil
}

func (r *Router) runSingleAttempt(
	attemptCtx context.Context,
	attemptReq Request,
	attemptNum int,
	out chan<- StreamEvent,
) (emittedPartial bool, completed bool, err error) {
	upstream, sErr := r.transport.Stream(attemptCtx, attemptReq)
	if sErr != nil {
		return false, false, sErr
	}

	for ev := range upstream {
		if ev.Attempt == 0 {
			ev.Attempt = attemptNum
		}
		switch ev.Type {
		case StreamThinkingDelta, StreamTextDelta, StreamToolCall, StreamCompaction:
			emittedPartial = true
			if !emitEvent(attemptCtx, out, ev) {
				if attemptCtx.Err() != nil {
					return emittedPartial, false, attemptCtx.Err()
				}
				return emittedPartial, false, context.Canceled
			}
		case StreamDone:
			emitEvent(context.Background(), out, ev)
			return emittedPartial, true, nil
		case StreamError:
			if ev.Err != nil {
				return emittedPartial, false, ev.Err
			}
			return emittedPartial, false, errors.New("upstream stream error")
		}
	}

	if attemptCtx.Err() != nil {
		return emittedPartial, false, attemptCtx.Err()
	}
	return emittedPartial, false, io.ErrUnexpectedEOF
}

func (r *Router) prepareRouteCredentials(
	ctx context.Context,
	route ResolvedRoute,
	explicitKey string,
) (config.ProviderConfig, string, []string, error) {
	prov := cloneProviderConfig(route.Provider)
	keySource := prov.SourceFile
	if route.apiKeySourceFile != "" {
		keySource = route.apiKeySourceFile
	}
	allowCmdKey := r.isTrustedGlobalSource(keySource)
	allowCmdHeaders := r.isTrustedGlobalSource(prov.SourceFile)

	var secretsList []string
	apiKey := explicitKey
	if apiKey != "" {
		secretsList = append(secretsList, apiKey)
	} else if ref := strings.TrimSpace(prov.APIKeyRef); ref != "" {
		if r.secrets == nil {
			return prov, "", nil, errors.New("llm router: secret resolver is nil")
		}
		resolved, err := r.secrets.Resolve(ctx, ref, allowCmdKey)
		if err != nil {
			return prov, "", secretsList, err
		}
		apiKey = resolved
		if apiKey != "" {
			secretsList = append(secretsList, apiKey)
		}
	}

	if len(prov.Headers) > 0 && r.secrets != nil {
		for k, v := range prov.Headers {
			trimmed := strings.TrimSpace(v)
			if strings.HasPrefix(trimmed, "env:") ||
				strings.HasPrefix(trimmed, "keychain:") ||
				strings.HasPrefix(trimmed, "cmd:") {
				resolvedVal, err := r.secrets.Resolve(ctx, trimmed, allowCmdHeaders)
				if err != nil {
					return prov, apiKey, secretsList, err
				}
				prov.Headers[k] = resolvedVal
				if resolvedVal != "" {
					secretsList = append(secretsList, resolvedVal)
				}
			}
		}
	}

	return prov, apiKey, secretsList, nil
}

// isTrustedGlobalSource 严格校验配置来源是否位于全局 ~/.gorness/ 保险库内部（或内存默认配置），阻断外部项目覆盖层执行 cmd:。
func (r *Router) isTrustedGlobalSource(sourceFile string) bool {
	trimmed := strings.TrimSpace(sourceFile)
	if trimmed == "" {
		return true
	}
	if r == nil || r.cfg == nil {
		return false
	}

	vaultRoot := filepath.Clean(r.cfg.Paths().Root)
	if vaultRoot == "" {
		return false
	}

	canonVault := vaultRoot
	if resolvedVault, err := filepath.EvalSymlinks(vaultRoot); err == nil {
		canonVault = filepath.Clean(resolvedVault)
	}

	cleanSrc := filepath.Clean(trimmed)
	if resolvedSrc, err := filepath.EvalSymlinks(cleanSrc); err == nil {
		cleanSrc = filepath.Clean(resolvedSrc)
		return isWithinVaultRoot(cleanSrc, canonVault)
	}
	return isWithinVaultRoot(cleanSrc, vaultRoot) || isWithinVaultRoot(cleanSrc, canonVault)
}

func isWithinVaultRoot(path, root string) bool {
	if path == root {
		return true
	}
	if root == string(filepath.Separator) {
		return strings.HasPrefix(path, root)
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

func (r *Router) scrubWithSecrets(err error, extraSecrets ...string) error {
	if err == nil {
		return nil
	}
	all := make([]string, 0, len(extraSecrets)+4)
	all = append(all, extraSecrets...)
	if r != nil && r.secrets != nil {
		if ks, ok := r.secrets.(interface{ KnownSecrets() []string }); ok {
			all = append(all, ks.KnownSecrets()...)
		}
	}
	return sanitizeError(err, all...)
}

func (r *Router) sleepBackoff(ctx context.Context, retryIdx int) bool {
	if r == nil || r.backoff == nil {
		return ctx.Err() == nil
	}
	d := r.backoff(retryIdx)
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func cloneProviderConfig(p config.ProviderConfig) config.ProviderConfig {
	cp := p
	if p.Headers != nil {
		cp.Headers = make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			cp.Headers[k] = v
		}
	}
	if p.Models != nil {
		cp.Models = make(map[string]config.ModelSpec, len(p.Models))
		for k, v := range p.Models {
			cp.Models[k] = v
		}
	}
	return cp
}
