package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gorness.dev/gorness/internal/alwayson"
	"gorness.dev/gorness/internal/config"
	"gorness.dev/gorness/internal/core"
	"gorness.dev/gorness/internal/fleet"
	"gorness.dev/gorness/internal/store"
	"gorness.dev/gorness/internal/tools"
	"gorness.dev/gorness/web"
)

const DefaultHeartbeatInterval = 4 * time.Second

type Deps struct {
	Config            *config.Manager
	Engine            *core.Engine
	AlwaysOn          *alwayson.Service
	Fleet             *fleet.Manager
	Opener            OpenerFunc
	Now               func() time.Time
	HeartbeatInterval time.Duration
	Version           string
}

// Server 承载 Gorness 本机回环 HTTP + 4s 心跳 SSE 服务、隔离 HTML 预览壳、安全副本打开器与内嵌 Apple HIG 工作台。
type Server struct {
	cfg               *config.Manager
	engine            *core.Engine
	alwaysOn          *alwayson.Service
	fallbackMemory    *alwayson.MemoryStore
	fleet             *fleet.Manager
	previews          *PreviewStore
	opener            *ExternalOpener
	now               func() time.Time
	heartbeatInterval time.Duration
	version           string

	configMu    sync.Mutex
	lastDiag    atomic.Pointer[config.ReloadDiagnostic]
	reloadUnsub func()

	bgCtx    context.Context
	bgCancel context.CancelFunc
	handler  http.Handler
}

func NewServer(deps Deps) *Server {
	nowFn := deps.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	hb := deps.HeartbeatInterval
	if hb <= 0 {
		hb = DefaultHeartbeatInterval
	}
	ver := strings.TrimSpace(deps.Version)
	if ver == "" {
		ver = config.Version
	}

	var paths config.VaultPaths
	if deps.Config != nil {
		paths = deps.Config.Paths()
	} else if deps.Engine != nil && deps.Engine.Config() != nil {
		deps.Config = deps.Engine.Config()
		paths = deps.Config.Paths()
	}

	var fallbackMem *alwayson.MemoryStore
	if strings.TrimSpace(paths.Root) != "" {
		fallbackMem = alwayson.NewMemoryStore(paths)
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())

	s := &Server{
		cfg:               deps.Config,
		engine:            deps.Engine,
		alwaysOn:          deps.AlwaysOn,
		fallbackMemory:    fallbackMem,
		fleet:             deps.Fleet,
		previews:          NewPersistentPreviewStore(paths, nowFn),
		opener:            NewExternalOpener(paths, deps.Opener),
		now:               nowFn,
		heartbeatInterval: hb,
		version:           ver,
		bgCtx:             bgCtx,
		bgCancel:          bgCancel,
	}

	if s.cfg != nil {
		s.reloadUnsub = s.cfg.OnReload(func(snap *config.Snapshot) {
			s.lastDiag.Store(nil)
			if s.engine != nil {
				s.engine.PublishSystemEvent(core.EventConfigReloaded, map[string]any{
					"snapshot":  sanitizeConfigSnapshot(snap),
					"timestamp": s.now().UTC(),
				})
			}
		})
		_ = s.cfg.Watch(bgCtx, func(_ *config.Snapshot, diag *config.ReloadDiagnostic) {
			s.lastDiag.Store(diag)
			if diag != nil && s.engine != nil {
				s.engine.PublishSystemEvent(core.EventConfigDiagnostic, diag)
			}
		})
	}

	s.handler = LoopbackGuard(s.buildMux())
	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Previews() *PreviewStore {
	return s.previews
}

func (s *Server) Close() error {
	if s != nil {
		if s.reloadUnsub != nil {
			s.reloadUnsub()
		}
		if s.bgCancel != nil {
			s.bgCancel()
		}
	}
	return nil
}

func (s *Server) buildMux() http.Handler {
	mux := http.NewServeMux()

	// Health, Bootstrap & Global Live Event Stream
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("GET /api/events/stream", s.handleGlobalEventsStream)

	// Config & Live ~/.gorness/ TOML/MD Studio
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config/global", s.handleUpdateGlobalConfig)
	mux.HandleFunc("POST /api/config/providers", s.handleUpsertProvider)
	mux.HandleFunc("DELETE /api/config/providers/{id}", s.handleDeleteProvider)
	mux.HandleFunc("POST /api/config/agents", s.handleUpsertAgent)
	mux.HandleFunc("DELETE /api/config/agents/{id}", s.handleDeleteAgent)
	mux.HandleFunc("POST /api/config/workflows", s.handleUpsertWorkflow)
	mux.HandleFunc("DELETE /api/config/workflows/{id}", s.handleDeleteWorkflow)
	mux.HandleFunc("PUT /api/config/rules", s.handleUpsertRules)
	mux.HandleFunc("GET /api/config/hooks", s.handleGetHooks)
	mux.HandleFunc("PUT /api/config/hooks", s.handleUpsertHooks)
	mux.HandleFunc("POST /api/config/mcp", s.handleUpsertMCP)
	mux.HandleFunc("DELETE /api/config/mcp/{id}", s.handleDeleteMCP)
	mux.HandleFunc("POST /api/config/secrets", s.handleSetSecret)
	mux.HandleFunc("DELETE /api/config/secrets/{key}", s.handleDeleteSecret)
	mux.HandleFunc("GET /api/config/files", s.handleGetConfigFiles)
	mux.HandleFunc("PUT /api/config/files", s.handleSaveConfigFile)

	// 4-Layer Pointer Memory & Candidate Skills
	mux.HandleFunc("GET /api/memory", s.handleGetMemory)
	mux.HandleFunc("PUT /api/memory/layer", s.handleUpdateMemoryLayer)
	mux.HandleFunc("GET /api/memory/notes/{slug}", s.handleGetMemoryNote)
	mux.HandleFunc("PUT /api/memory/notes/{slug}", s.handleSaveMemoryNote)
	mux.HandleFunc("DELETE /api/memory/notes/{slug}", s.handleDeleteMemoryNote)
	mux.HandleFunc("GET /api/skills/candidates", s.handleListSkillCandidates)
	mux.HandleFunc("POST /api/skills/candidates/{slug}/promote", s.handlePromoteSkillCandidate)
	mux.HandleFunc("DELETE /api/skills/candidates/{slug}", s.handleDeleteSkillCandidate)

	// Workspaces
	mux.HandleFunc("GET /api/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("POST /api/workspaces", s.handleRegisterWorkspace)
	mux.HandleFunc("GET /api/workspaces/suggestions", s.handleWorkspaceSuggestions)
	mux.HandleFunc("POST /api/workspaces/pick-folder", s.handlePickWorkspaceFolder)
	mux.HandleFunc("GET /api/workspaces/{wsID}", s.handleGetWorkspace)
	mux.HandleFunc("PATCH /api/workspaces/{wsID}", s.handleUpdateWorkspace)
	mux.HandleFunc("POST /api/workspaces/{wsID}/trust", s.handleTrustWorkspace)
	mux.HandleFunc("GET /api/workspaces/{wsID}/grants", s.handleListWorkspaceGrants)
	mux.HandleFunc("DELETE /api/workspaces/{wsID}/grants", s.handleDeleteWorkspaceGrants)
	mux.HandleFunc("GET /api/workspaces/{wsID}/files", s.handleListWorkspaceFiles)
	mux.HandleFunc("GET /api/workspaces/{wsID}/file", s.handleReadWorkspaceFile)
	mux.HandleFunc("GET /api/workspaces/{wsID}/skills", s.handleListWorkspaceSkills)
	mux.HandleFunc("GET /api/workspaces/{wsID}/artifacts", s.handleListWorkspaceArtifacts)
	mux.HandleFunc("GET /api/workspaces/{wsID}/memory/progress", s.handleListWorkspaceProgress)
	mux.HandleFunc("GET /api/workspaces/{wsID}/memory/progress/{slug}", s.handleGetWorkspaceProgress)
	mux.HandleFunc("PUT /api/workspaces/{wsID}/memory/progress/{slug}", s.handleSaveWorkspaceProgress)
	mux.HandleFunc("DELETE /api/workspaces/{wsID}/memory/progress/{slug}", s.handleDeleteWorkspaceProgress)

	// Sessions
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/search", s.handleSearchSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("PATCH /api/sessions/{id}", s.handleUpdateSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/fork", s.handleForkSession)
	mux.HandleFunc("POST /api/sessions/{id}/rewind", s.handleRewindSession)
	mux.HandleFunc("POST /api/sessions/{id}/compact", s.handleCompactSession)
	mux.HandleFunc("POST /api/sessions/{id}/move", s.handleMoveSession)
	mux.HandleFunc("GET /api/sessions/{id}/diff", s.handleDiffSession)
	mux.HandleFunc("POST /api/sessions/{id}/land", s.handleLandSession)
	mux.HandleFunc("GET /api/sessions/{id}/export", s.handleDownloadSessionExport)
	mux.HandleFunc("POST /api/sessions/{id}/export", s.handleExportSession)

	// Runs & Approvals
	mux.HandleFunc("GET /api/runs", s.handleListActiveRuns)
	mux.HandleFunc("POST /api/runs/stream", s.handleStartRunStream)
	mux.HandleFunc("GET /api/runs/{id}/stream", s.handleAttachRunStream)
	mux.HandleFunc("POST /api/runs/{id}/steer", s.handleSteerRun)
	mux.HandleFunc("POST /api/runs/{id}/stop", s.handleStopRun)
	mux.HandleFunc("GET /api/approvals", s.handleListApprovals)
	mux.HandleFunc("POST /api/approvals/{id}/resolve", s.handleResolveApproval)

	// Always-On Sleep-Wake Primitives
	mux.HandleFunc("GET /api/alwayson", s.handleAlwaysOnState)
	mux.HandleFunc("POST /api/alwayson/webhook/{source}", s.handleAlwaysOnWebhook)
	mux.HandleFunc("GET /api/alwayson/timers", s.handleListTimers)
	mux.HandleFunc("POST /api/alwayson/timers", s.handleCreateTimer)
	mux.HandleFunc("PATCH /api/alwayson/timers/{id}", s.handleUpdateTimer)
	mux.HandleFunc("DELETE /api/alwayson/timers/{id}", s.handleDeleteTimer)
	mux.HandleFunc("POST /api/alwayson/timers/{id}/trigger", s.handleTriggerTimer)
	mux.HandleFunc("POST /api/alwayson/timers/tick", s.handleTickTimers)
	mux.HandleFunc("GET /api/alwayson/deferred", s.handleListDeferred)
	mux.HandleFunc("POST /api/alwayson/deferred/{id}/resolve", s.handleResolveDeferred)
	mux.HandleFunc("GET /api/alwayson/outbox", s.handleListOutbox)
	mux.HandleFunc("POST /api/alwayson/outbox/ack", s.handleAckOutbox)
	mux.HandleFunc("POST /api/alwayson/dream", s.handleRunDream)

	// Visual Multi-Agent Fleet Orchestration
	mux.HandleFunc("GET /api/fleet/missions", s.handleListMissions)
	mux.HandleFunc("POST /api/fleet/missions", s.handleStartMission)
	mux.HandleFunc("GET /api/fleet/missions/{id}", s.handleGetMission)
	mux.HandleFunc("DELETE /api/fleet/missions/{id}", s.handleDeleteMission)
	mux.HandleFunc("GET /api/fleet/missions/{id}/stream", s.handleStreamMission)
	mux.HandleFunc("GET /api/fleet/missions/{id}/nodes/{nodeID}/diff", s.handleInspectNodeDiff)
	mux.HandleFunc("POST /api/fleet/missions/{id}/nodes/{nodeID}/steer", s.handleSteerNode)
	mux.HandleFunc("POST /api/fleet/missions/{id}/stop", s.handleStopMission)

	// Safe Copy-Open & Isolated HTML Artifact Previews
	mux.HandleFunc("POST /api/open-safe", s.handleOpenSafe)
	mux.HandleFunc("GET /api/previews", s.handleListPreviews)
	mux.HandleFunc("POST /api/previews", s.handleCreatePreview)
	mux.HandleFunc("GET /preview/{token}", s.handleServePreview)

	// Embedded Apple HIG Workbench Static Assets
	fileServer := http.FileServer(http.FS(web.Assets))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusNotFound, "api endpoint not found")
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			raw, err := fs.ReadFile(web.Assets, "index.html")
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "missing embedded index.html")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(raw)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	return mux
}

// ============================================================================
// Health, Bootstrap & Global SSE
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	var vaultRoot string
	if s.cfg != nil {
		vaultRoot = s.cfg.Paths().Root
	}
	var activeRuns, pendingApprovals int
	if s.engine != nil {
		activeRuns = len(s.engine.ActiveRuns())
		if s.engine.Broker() != nil {
			pendingApprovals = len(s.engine.Broker().ListPending(""))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"version":           s.version,
		"pid":               os.Getpid(),
		"vault_root":        vaultRoot,
		"active_runs":       activeRuns,
		"pending_approvals": pendingApprovals,
		"timestamp":         s.now().UTC(),
	})
}

func (s *Server) handleBootstrap(w http.ResponseWriter, _ *http.Request) {
	var snap *config.Snapshot
	var vaultRoot string
	if s.cfg != nil {
		snap = sanitizeConfigSnapshot(s.cfg.Current())
		vaultRoot = s.cfg.Paths().Root
	}

	var workspaces []*store.Workspace
	var sessions []*store.SessionMeta
	var activeRuns []core.ActiveRunSummary
	var approvals []core.PendingApproval
	if s.engine != nil {
		workspaces = s.engine.Workspaces().List()
		sessions = s.engine.Sessions().List("", true)
		activeRuns = s.engine.ActiveRuns()
		if s.engine.Broker() != nil {
			approvals = s.engine.Broker().ListPending("")
		}
	}
	if workspaces == nil {
		workspaces = []*store.Workspace{}
	}
	if sessions == nil {
		sessions = []*store.SessionMeta{}
	}
	if activeRuns == nil {
		activeRuns = []core.ActiveRunSummary{}
	}
	if approvals == nil {
		approvals = []core.PendingApproval{}
	}

	var aoState alwayson.AlwaysOnState
	if s.alwaysOn != nil {
		aoState = s.alwaysOn.StateSnapshot()
	}

	missions := []*fleet.MissionSnapshot{}
	if s.fleet != nil {
		for _, ws := range workspaces {
			if list, err := s.fleet.ListMissions(ws.ID); err == nil {
				missions = append(missions, list...)
			}
		}
		sort.Slice(missions, func(i, j int) bool {
			return missions[i].UpdatedAt.After(missions[j].UpdatedAt)
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":           s.version,
		"vault_root":        vaultRoot,
		"config":            snap,
		"diagnostic":        s.lastDiag.Load(),
		"config_files":      s.listVaultConfigRelPaths(),
		"workspaces":        workspaces,
		"sessions":          sessions,
		"active_runs":       activeRuns,
		"pending_approvals": approvals,
		"alwayson":          aoState,
		"skill_candidates":  s.listSkillCandidatesSnapshot(),
		"missions":          missions,
		"previews":          s.previews.List(""),
	})
}

func (s *Server) handleGlobalEventsStream(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "engine unavailable")
		return
	}
	flusher, ok := prepareSSEHeaders(w)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	ch, unsub := s.engine.SubscribeGlobal()
	defer unsub()

	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	_ = writeSSEFrame(w, flusher, "connected", map[string]any{
		"type":      "connected",
		"timestamp": s.now().UTC(),
	})

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.bgCtx.Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if err := writeSSEFrame(w, flusher, ev.Type, ev); err != nil {
				return
			}
		case t := <-ticker.C:
			if err := writeSSEFrame(w, flusher, "heartbeat", map[string]any{
				"type":      "heartbeat",
				"timestamp": t.UTC(),
			}); err != nil {
				return
			}
		}
	}
}

// ============================================================================
// Config & Agent-Model Matrix Endpoints (/api/config/*)
// ============================================================================

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot":   sanitizeConfigSnapshot(s.cfg.Current()),
		"diagnostic": s.lastDiag.Load(),
		"files":      s.listVaultConfigRelPaths(),
	})
}

func (s *Server) handleUpdateGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var patch map[string]any
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	cur := s.cfg.Current().Global
	if v, ok := patch["theme"].(string); ok && strings.TrimSpace(v) != "" {
		cur.Theme = strings.TrimSpace(v)
		cur.UITheme = strings.TrimSpace(v)
	}
	if v, ok := patch["ui_theme"].(string); ok && strings.TrimSpace(v) != "" {
		cur.UITheme = strings.TrimSpace(v)
		cur.Theme = strings.TrimSpace(v)
	}
	if v, ok := patch["language"].(string); ok && strings.TrimSpace(v) != "" {
		cur.Language = strings.TrimSpace(v)
		cur.UILang = strings.TrimSpace(v)
	}
	if v, ok := patch["ui_lang"].(string); ok && strings.TrimSpace(v) != "" {
		cur.UILang = strings.TrimSpace(v)
		cur.Language = strings.TrimSpace(v)
	}
	if v, ok := patch["default_provider"].(string); ok && strings.TrimSpace(v) != "" {
		cur.DefaultProvider = strings.TrimSpace(v)
	}
	if v, ok := patch["default_model"].(string); ok && strings.TrimSpace(v) != "" {
		cur.DefaultModel = strings.TrimSpace(v)
	}
	if v, ok := patch["default_effort"].(string); ok && strings.TrimSpace(v) != "" {
		cur.DefaultEffort = strings.TrimSpace(v)
	}
	if v, ok := patch["permission_mode"].(string); ok && strings.TrimSpace(v) != "" {
		cur.PermissionMode = core.NormalizePermMode(v)
	}
	if v, ok := patch["sandbox_mode"].(string); ok && strings.TrimSpace(v) != "" {
		cur.SandboxMode = string(tools.NormalizeSandboxMode(v))
	}
	if v, ok := patch["max_concurrency"].(float64); ok && int(v) > 0 {
		cur.MaxConcurrency = int(v)
	}
	if v, ok := patch["max_tool_concurrency"].(float64); ok && int(v) > 0 {
		cur.MaxToolConcurrency = int(v)
	}
	if v, ok := patch["max_steps"].(float64); ok && int(v) > 0 {
		cur.MaxSteps = int(v)
	}
	if v, ok := patch["inbox_debounce_ms"].(float64); ok && int(v) >= 0 {
		cur.InboxDebounceMS = int(v)
	}
	if !isLoopbackHostname(cur.Host) {
		cur.Host = "127.0.0.1"
	}

	encoded, err := toml.Marshal(cur)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal config.toml: %v", err))
		return
	}
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"config.toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(s.cfg.Paths().ConfigFile(), encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

type providerPatchInput struct {
	ID         string                       `json:"id"`
	Name       *string                      `json:"name"`
	Protocol   *string                      `json:"protocol"`
	BaseURL    *string                      `json:"base_url"`
	APIKeyRef  *string                      `json:"api_key"`
	Headers    *map[string]string           `json:"headers"`
	TimeoutSec *int                         `json:"timeout_sec"`
	Models     *map[string]config.ModelSpec `json:"models"`
}

func (s *Server) handleUpsertProvider(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var patch providerPatchInput
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	provID := strings.TrimSpace(patch.ID)
	if err := store.ValidateSafeID(provID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid provider id: %v", err))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	var doc struct {
		Providers map[string]config.ProviderConfig `toml:"providers"`
	}
	if raw, err := os.ReadFile(s.cfg.Paths().ProvidersFile()); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		_ = toml.Unmarshal(raw, &doc)
	}
	if doc.Providers == nil {
		doc.Providers = make(map[string]config.ProviderConfig)
	}

	var merged config.ProviderConfig
	existing, hasExisting := doc.Providers[provID]
	if hasExisting {
		merged = existing
		if existing.Headers != nil {
			merged.Headers = make(map[string]string, len(existing.Headers))
			for hk, hv := range existing.Headers {
				merged.Headers[hk] = hv
			}
		}
		if existing.Models != nil {
			merged.Models = make(map[string]config.ModelSpec, len(existing.Models))
			for mk, mv := range existing.Models {
				merged.Models[mk] = mv
			}
		}
	}
	merged.ID = provID

	if patch.Name != nil && strings.TrimSpace(*patch.Name) != "" {
		merged.Name = strings.TrimSpace(*patch.Name)
	}
	if strings.TrimSpace(merged.Name) == "" {
		merged.Name = provID
	}
	if patch.Protocol != nil && strings.TrimSpace(*patch.Protocol) != "" {
		merged.Protocol = strings.TrimSpace(*patch.Protocol)
	}
	if strings.TrimSpace(merged.Protocol) == "" {
		merged.Protocol = "openai-compat"
	}
	if patch.BaseURL != nil && strings.TrimSpace(*patch.BaseURL) != "" {
		merged.BaseURL = strings.TrimSpace(*patch.BaseURL)
	}
	if patch.APIKeyRef != nil {
		ref := strings.TrimSpace(*patch.APIKeyRef)
		if ref != "" && ref != "[REDACTED]" {
			merged.APIKeyRef = ref
		}
	}
	if patch.Headers != nil {
		if len(*patch.Headers) == 0 {
			merged.Headers = nil
		} else {
			if merged.Headers == nil {
				merged.Headers = make(map[string]string, len(*patch.Headers))
			}
			for hk, hv := range *patch.Headers {
				if hasExisting && strings.TrimSpace(hv) == "[REDACTED]" {
					if prev, ok := existing.Headers[hk]; ok {
						merged.Headers[hk] = prev
						continue
					}
				}
				merged.Headers[hk] = hv
			}
		}
	}
	if patch.TimeoutSec != nil && *patch.TimeoutSec > 0 {
		merged.TimeoutSec = *patch.TimeoutSec
	}
	if patch.Models != nil {
		if len(*patch.Models) == 0 {
			merged.Models = map[string]config.ModelSpec{}
		} else {
			if merged.Models == nil {
				merged.Models = make(map[string]config.ModelSpec, len(*patch.Models))
			}
			for mk, mv := range *patch.Models {
				if mv.ID == "" {
					mv.ID = mk
				}
				merged.Models[mk] = mv
			}
		}
	}

	doc.Providers[provID] = merged

	encoded, err := toml.Marshal(doc)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal providers.toml: %v", err))
		return
	}
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"providers.toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(s.cfg.Paths().ProvidersFile(), encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

type agentPatchInput struct {
	ID              string    `json:"id"`
	Name            *string   `json:"name"`
	Description     *string   `json:"description"`
	Provider        *string   `json:"provider"`
	Model           *string   `json:"model"`
	Effort          *string   `json:"effort"`
	Fallbacks       *[]string `json:"fallbacks"`
	AllowedTools    *[]string `json:"allowed_tools"`
	DisallowedTools *[]string `json:"disallowed_tools"`
	ReadOnly        *bool     `json:"read_only"`
	APIKeyRef       *string   `json:"api_key"`
	SystemPrompt    *string   `json:"system_prompt"`
}

type agentFrontmatterDoc struct {
	ID              string    `toml:"id"`
	Name            string    `toml:"name,omitempty"`
	Description     string    `toml:"description,omitempty"`
	Provider        string    `toml:"provider"`
	Model           string    `toml:"model"`
	Effort          string    `toml:"effort,omitempty"`
	Fallbacks       *[]string `toml:"fallbacks"`
	AllowedTools    *[]string `toml:"allowed_tools"`
	DisallowedTools *[]string `toml:"disallowed_tools"`
	ReadOnly        bool      `toml:"read_only,omitempty"`
	APIKeyRef       string    `toml:"api_key,omitempty"`
}

func (s *Server) handleUpsertAgent(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var patch agentPatchInput
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	agentID := strings.TrimSpace(patch.ID)
	if err := store.ValidateSafeID(agentID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid agent id: %v", err))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	var merged config.AgentProfile
	if snap := s.cfg.Current().Clone(); snap != nil {
		if existing, ok := snap.Agents[agentID]; ok {
			merged = existing
		}
	}
	merged.ID = agentID

	if patch.Name != nil && strings.TrimSpace(*patch.Name) != "" {
		merged.Name = strings.TrimSpace(*patch.Name)
	}
	if strings.TrimSpace(merged.Name) == "" {
		merged.Name = agentID
	}
	if patch.Description != nil {
		merged.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Provider != nil && strings.TrimSpace(*patch.Provider) != "" {
		merged.Provider = strings.TrimSpace(*patch.Provider)
	}
	if patch.Model != nil && strings.TrimSpace(*patch.Model) != "" {
		merged.Model = strings.TrimSpace(*patch.Model)
	}
	if patch.Effort != nil && strings.TrimSpace(*patch.Effort) != "" {
		merged.Effort = strings.TrimSpace(*patch.Effort)
	}
	if patch.Fallbacks != nil {
		cp := make([]string, len(*patch.Fallbacks))
		copy(cp, *patch.Fallbacks)
		merged.Fallbacks = cp
	}
	if patch.AllowedTools != nil {
		cp := make([]string, len(*patch.AllowedTools))
		copy(cp, *patch.AllowedTools)
		merged.AllowedTools = cp
	}
	if patch.DisallowedTools != nil {
		cp := make([]string, len(*patch.DisallowedTools))
		copy(cp, *patch.DisallowedTools)
		merged.DisallowedTools = cp
	}
	if patch.ReadOnly != nil {
		merged.ReadOnly = *patch.ReadOnly
	}
	if patch.APIKeyRef != nil {
		ref := strings.TrimSpace(*patch.APIKeyRef)
		if ref != "[REDACTED]" {
			merged.APIKeyRef = ref
		}
	}
	if patch.SystemPrompt != nil && strings.TrimSpace(*patch.SystemPrompt) != "" {
		merged.SystemPrompt = strings.TrimSpace(*patch.SystemPrompt)
	}
	if strings.TrimSpace(merged.SystemPrompt) == "" {
		merged.SystemPrompt = fmt.Sprintf("You are the %s agent in Gorness.", merged.Name)
	}

	fmDoc := agentFrontmatterDoc{
		ID:          merged.ID,
		Name:        merged.Name,
		Description: merged.Description,
		Provider:    merged.Provider,
		Model:       merged.Model,
		Effort:      merged.Effort,
		ReadOnly:    merged.ReadOnly,
		APIKeyRef:   merged.APIKeyRef,
	}
	if merged.Fallbacks != nil {
		fb := append([]string{}, merged.Fallbacks...)
		fmDoc.Fallbacks = &fb
	}
	if merged.AllowedTools != nil {
		at := append([]string{}, merged.AllowedTools...)
		fmDoc.AllowedTools = &at
	}
	if merged.DisallowedTools != nil {
		dt := append([]string{}, merged.DisallowedTools...)
		fmDoc.DisallowedTools = &dt
	}

	fmBytes, err := toml.Marshal(fmDoc)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal agent frontmatter: %v", err))
		return
	}
	var doc bytes.Buffer
	doc.WriteString("+++\n")
	doc.Write(bytes.TrimRight(fmBytes, "\n"))
	doc.WriteString("\n+++\n")
	doc.WriteString(strings.TrimSpace(merged.SystemPrompt))
	doc.WriteByte('\n')

	targetPath := filepath.Join(s.cfg.Paths().AgentsDir(), agentID+".md")
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"agents/" + agentID + ".md": doc.Bytes()}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(targetPath, doc.Bytes(), 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

func (s *Server) handleUpsertWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var in config.WorkflowTemplate
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	wfID := strings.TrimSpace(in.ID)
	if err := store.ValidateSafeID(wfID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid workflow id: %v", err))
		return
	}
	if _, err := fleet.ParseTopologyKind(firstNonEmptyStr(in.Topology, wfID)); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	in.ID = wfID
	if strings.TrimSpace(in.Name) == "" {
		in.Name = wfID
	}
	if strings.TrimSpace(in.Topology) == "" {
		in.Topology = wfID
	}

	encoded, err := toml.Marshal(in)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal workflow: %v", err))
		return
	}
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"workflows/" + wfID + ".toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	targetPath := filepath.Join(s.cfg.Paths().WorkflowsDir(), wfID+".toml")
	if err := store.AtomicWriteFile(targetPath, encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	provID := strings.TrimSpace(r.PathValue("id"))
	if err := store.ValidateSafeID(provID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid provider id: %v", err))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	var doc struct {
		Providers map[string]config.ProviderConfig `toml:"providers"`
	}
	if raw, err := os.ReadFile(s.cfg.Paths().ProvidersFile()); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		_ = toml.Unmarshal(raw, &doc)
	}
	if len(doc.Providers) <= 1 {
		writeJSONError(w, http.StatusBadRequest, "cannot delete the last remaining provider")
		return
	}
	delete(doc.Providers, provID)

	encoded, err := toml.Marshal(doc)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal providers.toml: %v", err))
		return
	}
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"providers.toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(s.cfg.Paths().ProvidersFile(), encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

type rulesPatchInput struct {
	AllowCommands  *[]string                `json:"allow_commands"`
	AskCommands    *[]string                `json:"ask_commands"`
	DenyCommands   *[]string                `json:"deny_commands"`
	AllowPaths     *[]string                `json:"allow_paths"`
	DenyPaths      *[]string                `json:"deny_paths"`
	AllowedDomains *[]string                `json:"allowed_domains"`
	Rules          *[]config.PermissionRule `json:"rules"`
}

func (s *Server) handleUpsertRules(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var patch rulesPatchInput
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	var merged config.RuleSet
	if cur := s.cfg.Current(); cur != nil {
		merged = cur.Clone().Rules
	}
	cleanStrList := func(in []string) []string {
		seen := make(map[string]bool, len(in))
		out := make([]string, 0, len(in))
		for _, item := range in {
			v := strings.TrimSpace(item)
			if v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
		return out
	}

	if patch.AllowCommands != nil {
		merged.AllowCommands = cleanStrList(*patch.AllowCommands)
	}
	if patch.AskCommands != nil {
		merged.AskCommands = cleanStrList(*patch.AskCommands)
	}
	if patch.DenyCommands != nil {
		merged.DenyCommands = cleanStrList(*patch.DenyCommands)
	}
	if patch.AllowPaths != nil {
		merged.AllowPaths = cleanStrList(*patch.AllowPaths)
	}
	if patch.DenyPaths != nil {
		merged.DenyPaths = cleanStrList(*patch.DenyPaths)
	}
	if patch.AllowedDomains != nil {
		merged.AllowedDomains = cleanStrList(*patch.AllowedDomains)
	}
	if patch.Rules != nil {
		cp := make([]config.PermissionRule, len(*patch.Rules))
		copy(cp, *patch.Rules)
		merged.Rules = cp
	}

	encoded, err := toml.Marshal(merged)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal rules.toml: %v", err))
		return
	}
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"rules.toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(s.cfg.Paths().RulesFile(), encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

type mcpPatchInput struct {
	ID         string             `json:"id"`
	Transport  *string            `json:"transport"`
	Command    *string            `json:"command"`
	Args       *[]string          `json:"args"`
	URL        *string            `json:"url"`
	Env        *map[string]string `json:"env"`
	Headers    *map[string]string `json:"headers"`
	Enabled    *bool              `json:"enabled"`
	TimeoutSec *int               `json:"timeout_sec"`
}

func (s *Server) handleUpsertMCP(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var patch mcpPatchInput
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	srvID := strings.TrimSpace(patch.ID)
	if err := store.ValidateSafeID(srvID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid mcp server id: %v", err))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	var doc config.MCPConfig
	if raw, err := os.ReadFile(s.cfg.Paths().MCPFile()); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		_ = toml.Unmarshal(raw, &doc)
	}
	if doc.Servers == nil {
		doc.Servers = make(map[string]config.MCPServerConfig)
	}

	merged, hasExisting := doc.Servers[srvID]
	merged.ID = srvID
	if patch.Transport != nil && strings.TrimSpace(*patch.Transport) != "" {
		merged.Transport = strings.TrimSpace(*patch.Transport)
	}
	if strings.TrimSpace(merged.Transport) == "" {
		merged.Transport = "stdio"
	}
	if patch.Command != nil {
		merged.Command = strings.TrimSpace(*patch.Command)
	}
	if patch.Args != nil {
		cp := make([]string, len(*patch.Args))
		copy(cp, *patch.Args)
		merged.Args = cp
	}
	if patch.URL != nil {
		merged.URL = strings.TrimSpace(*patch.URL)
	}
	if patch.Env != nil {
		if len(*patch.Env) == 0 {
			merged.Env = nil
		} else {
			if merged.Env == nil {
				merged.Env = make(map[string]string, len(*patch.Env))
			}
			for k, v := range *patch.Env {
				if hasExisting && strings.TrimSpace(v) == "[REDACTED]" {
					continue
				}
				merged.Env[k] = v
			}
		}
	}
	if patch.Headers != nil {
		if len(*patch.Headers) == 0 {
			merged.Headers = nil
		} else {
			if merged.Headers == nil {
				merged.Headers = make(map[string]string, len(*patch.Headers))
			}
			for k, v := range *patch.Headers {
				if hasExisting && strings.TrimSpace(v) == "[REDACTED]" {
					continue
				}
				merged.Headers[k] = v
			}
		}
	}
	if patch.Enabled != nil {
		val := *patch.Enabled
		merged.Enabled = &val
	} else if merged.Enabled == nil {
		val := true
		merged.Enabled = &val
	}
	if patch.TimeoutSec != nil && *patch.TimeoutSec > 0 {
		merged.TimeoutSec = *patch.TimeoutSec
	} else if merged.TimeoutSec <= 0 {
		merged.TimeoutSec = 60
	}

	doc.Servers[srvID] = merged
	encoded, err := toml.Marshal(doc)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal mcp.toml: %v", err))
		return
	}
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"mcp.toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(s.cfg.Paths().MCPFile(), encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

func (s *Server) handleDeleteMCP(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	srvID := strings.TrimSpace(r.PathValue("id"))
	if err := store.ValidateSafeID(srvID); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid mcp server id: %v", err))
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	var doc config.MCPConfig
	if raw, err := os.ReadFile(s.cfg.Paths().MCPFile()); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		_ = toml.Unmarshal(raw, &doc)
	}
	if doc.Servers != nil {
		delete(doc.Servers, srvID)
	}
	encoded, err := toml.Marshal(doc)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("marshal mcp.toml: %v", err))
		return
	}
	if _, err := s.cfg.ValidateCandidate(map[string][]byte{"mcp.toml": encoded}); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.AtomicWriteFile(s.cfg.Paths().MCPFile(), encoded, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, err := s.cfg.Reload()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, sanitizeConfigSnapshot(snap))
}

func (s *Server) handleGetConfigFiles(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	rel := firstNonEmptyStr(r.URL.Query().Get("rel"), r.URL.Query().Get("rel_path"), r.URL.Query().Get("path"))
	if rel == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"files": s.listVaultConfigRelPaths(),
		})
		return
	}
	absPath, normRel, err := s.resolveEditableVaultFile(rel)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "config file not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rel_path": normRel,
		"path":     normRel,
		"content":  s.sanitizeRawConfigFileContent(string(raw)),
	})
}

func (s *Server) handleSaveConfigFile(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "config manager unavailable")
		return
	}
	var in struct {
		RelPath string `json:"rel_path"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	absPath, normRel, err := s.resolveEditableVaultFile(firstNonEmptyStr(in.RelPath, in.Path))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	rawBytes := []byte(in.Content)
	if strings.HasSuffix(normRel, ".toml") {
		var check map[string]any
		if err := toml.Unmarshal(rawBytes, &check); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("TOML syntax error in %s: %v", normRel, err))
			return
		}
		if normRel == "config.toml" {
			var g config.GlobalConfig
			if err := toml.Unmarshal(rawBytes, &g); err != nil {
				writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid config.toml: %v", err))
				return
			}
			if g.Host != "" && !isLoopbackHostname(g.Host) {
				writeJSONError(w, http.StatusBadRequest, "config.toml host must remain a loopback address (127.0.0.1 / localhost / ::1)")
				return
			}
		}
	} else if strings.HasPrefix(normRel, "agents/") || strings.HasPrefix(normRel, "skills/") {
		if _, _, err := config.ParseFrontmatterMD(rawBytes); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("frontmatter error in %s: %v", normRel, err))
			return
		}
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	if existingRaw, err := os.ReadFile(absPath); err == nil && len(existingRaw) > 0 {
		rawBytes = s.restoreRedactedConfigFileContent(normRel, existingRaw, rawBytes)
	}

	if _, err := s.cfg.ValidateCandidate(map[string][]byte{normRel: rawBytes}); err != nil {
		diag := &config.ReloadDiagnostic{
			File:      absPath,
			Message:   err.Error(),
			Timestamp: s.now().UTC(),
		}
		s.lastDiag.Store(diag)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := store.AtomicWriteFile(absPath, rawBytes, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snap, reloadErr := s.cfg.Reload()
	if reloadErr != nil {
		diag := &config.ReloadDiagnostic{
			File:      absPath,
			Message:   reloadErr.Error(),
			Timestamp: s.now().UTC(),
		}
		s.lastDiag.Store(diag)
		writeJSONError(w, http.StatusBadRequest, reloadErr.Error())
		return
	}
	s.lastDiag.Store(nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"rel_path": normRel,
		"snapshot": sanitizeConfigSnapshot(snap),
	})
}

func (s *Server) listVaultConfigRelPaths() []string {
	if s.cfg == nil {
		return nil
	}
	paths := s.cfg.Paths()
	out := []string{"config.toml", "providers.toml", "rules.toml", "mcp.toml"}
	if st, err := os.Lstat(paths.HooksFile()); err == nil && st.Mode()&os.ModeSymlink == 0 && st.Mode().IsRegular() {
		out = append(out, "hooks.toml")
	}

	appendDir := func(subDir, ext string) {
		entries, err := os.ReadDir(filepath.Join(paths.Root, subDir))
		if err != nil {
			return
		}
		var names []string
		for _, ent := range entries {
			if !ent.IsDir() && ent.Type()&os.ModeSymlink == 0 && strings.HasSuffix(ent.Name(), ext) {
				names = append(names, filepath.ToSlash(filepath.Join(subDir, ent.Name())))
			}
		}
		sort.Strings(names)
		out = append(out, names...)
	}
	appendSkillDirs := func(subDir string) {
		entries, err := os.ReadDir(filepath.Join(paths.Root, subDir))
		if err != nil {
			return
		}
		var names []string
		for _, ent := range entries {
			if !ent.IsDir() || ent.Type()&os.ModeSymlink != 0 || ent.Name() == "_candidates" {
				continue
			}
			if store.ValidateSafeID(ent.Name()) != nil {
				continue
			}
			skillMD := filepath.Join(paths.Root, subDir, ent.Name(), "SKILL.md")
			if st, sErr := os.Lstat(skillMD); sErr == nil && st.Mode()&os.ModeSymlink == 0 && st.Mode().IsRegular() {
				names = append(names, filepath.ToSlash(filepath.Join(subDir, ent.Name(), "SKILL.md")))
			}
		}
		sort.Strings(names)
		out = append(out, names...)
	}

	appendDir("agents", ".md")
	appendDir("workflows", ".toml")
	appendDir("memory", ".md")
	appendDir(filepath.Join("memory", "notes"), ".md")
	appendSkillDirs("skills")
	appendSkillDirs(filepath.Join("skills", "_candidates"))
	return out
}

func (s *Server) resolveEditableVaultFile(rel string) (absPath, normRel string, err error) {
	cleaned := filepath.ToSlash(strings.TrimSpace(rel))
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "" || strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, "..") {
		return "", "", errors.New("invalid vault config relative path")
	}

	ext := strings.ToLower(filepath.Ext(cleaned))
	if ext != ".toml" && ext != ".md" {
		return "", "", errors.New("vault zero-DB rule: only .toml and .md config files are editable")
	}

	allowed := false
	switch cleaned {
	case "config.toml", "providers.toml", "rules.toml", "mcp.toml", "hooks.toml":
		allowed = true
	default:
		parts := strings.Split(cleaned, "/")
		if len(parts) == 2 && (parts[0] == "agents" || parts[0] == "memory") && ext == ".md" && store.ValidateSafeID(strings.TrimSuffix(parts[1], ".md")) == nil {
			allowed = true
		} else if len(parts) == 2 && parts[0] == "workflows" && ext == ".toml" && store.ValidateSafeID(strings.TrimSuffix(parts[1], ".toml")) == nil {
			allowed = true
		} else if len(parts) == 3 && parts[0] == "memory" && parts[1] == "notes" && ext == ".md" && store.ValidateSafeID(strings.TrimSuffix(parts[2], ".md")) == nil {
			allowed = true
		} else if len(parts) == 3 && parts[0] == "skills" && parts[1] != "_candidates" && parts[2] == "SKILL.md" && store.ValidateSafeID(parts[1]) == nil {
			allowed = true
		} else if len(parts) == 4 && parts[0] == "skills" && parts[1] == "_candidates" && parts[3] == "SKILL.md" && store.ValidateSafeID(parts[2]) == nil {
			allowed = true
		}
	}
	if !allowed {
		return "", "", fmt.Errorf("path %q is not an editable ~/.gorness configuration file", cleaned)
	}

	cur := s.cfg.Paths().Root
	segs := strings.Split(cleaned, "/")
	for i := 0; i < len(segs)-1; i++ {
		cur = filepath.Join(cur, segs[i])
		if dirSt, lErr := os.Lstat(cur); lErr == nil && (dirSt.Mode()&os.ModeSymlink != 0 || !dirSt.IsDir()) {
			return "", "", errors.New("refusing to traverse symlink or non-directory in vault")
		}
	}

	full := filepath.Join(s.cfg.Paths().Root, filepath.FromSlash(cleaned))
	if st, lErr := os.Lstat(full); lErr == nil && (st.Mode()&os.ModeSymlink != 0 || st.IsDir()) {
		return "", "", errors.New("refusing to edit symlink or directory in vault")
	}
	return full, cleaned, nil
}

// ============================================================================
// Workspaces Endpoints (/api/workspaces/*)
// ============================================================================

func (s *Server) handleListWorkspaces(w http.ResponseWriter, _ *http.Request) {
	if s.engine == nil {
		writeJSON(w, http.StatusOK, []*store.Workspace{})
		return
	}
	writeJSON(w, http.StatusOK, s.engine.Workspaces().List())
}

func (s *Server) handleRegisterWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "engine unavailable")
		return
	}
	var in struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ws, err := s.engine.Workspaces().Register(in.Path, in.Name)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

type workspaceSuggestion struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsGit      bool   `json:"is_git"`
	Registered bool   `json:"registered"`
}

func (s *Server) handleWorkspaceSuggestions(w http.ResponseWriter, _ *http.Request) {
	homeDir := ""
	vaultRoot := ""
	if s.cfg != nil {
		vaultRoot = s.cfg.Paths().Root
		homeDir = filepath.Dir(vaultRoot)
	}
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	registeredPaths := make(map[string]bool)
	if s.engine != nil {
		for _, ws := range s.engine.Workspaces().List() {
			registeredPaths[filepath.Clean(ws.RootPath)] = true
		}
	}

	searchRoots := []string{
		filepath.Join(homeDir, "Documents", "MyProjectes"),
		filepath.Join(homeDir, "Documents", "Projects"),
		filepath.Join(homeDir, "Projects"),
		filepath.Join(homeDir, "Developer"),
		filepath.Join(homeDir, "Code"),
		filepath.Join(homeDir, "Workspace"),
		filepath.Join(homeDir, "Workspaces"),
		filepath.Join(homeDir, "repos"),
		filepath.Join(homeDir, "src"),
	}

	seen := make(map[string]bool)
	var suggestions []workspaceSuggestion

	for _, root := range searchRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
				continue
			}
			full := filepath.Clean(filepath.Join(root, ent.Name()))
			if full == filepath.Clean(vaultRoot) || seen[full] {
				continue
			}
			seen[full] = true
			_, gitErr := os.Stat(filepath.Join(full, ".git"))
			suggestions = append(suggestions, workspaceSuggestion{
				Name:       ent.Name(),
				Path:       full,
				IsGit:      gitErr == nil,
				Registered: registeredPaths[full],
			})
			if len(suggestions) >= 60 {
				break
			}
		}
		if len(suggestions) >= 60 {
			break
		}
	}

	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].IsGit != suggestions[j].IsGit {
			return suggestions[i].IsGit
		}
		return strings.ToLower(suggestions[i].Name) < strings.ToLower(suggestions[j].Name)
	})

	if suggestions == nil {
		suggestions = []workspaceSuggestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"suggestions": suggestions,
	})
}

func (s *Server) handlePickWorkspaceFolder(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", `POSIX path of (choose folder with prompt "选择要挂载到 Gorness 的项目文件夹 / Select Project Folder")`)
	out, err := cmd.Output()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	chosen := strings.TrimRight(strings.TrimSpace(string(out)), "/")
	if chosen == "" {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cancelled": false,
		"path":      chosen,
		"name":      filepath.Base(chosen),
	})
}

func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("wsID")
	ws, err := s.engine.Workspaces().Get(wsID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var warnings []string
	if s.cfg != nil {
		_, warnings, _ = s.cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace": ws,
		"warnings":  warnings,
	})
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("wsID")
	var patch struct {
		Name          *string           `json:"name"`
		PermMode      *string           `json:"perm_mode"`
		SandboxMode   *string           `json:"sandbox_mode"`
		RoleModels    map[string]string `json:"role_models"`
		TrustedHashes map[string]string `json:"trusted_hashes"`
	}
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.engine.Workspaces().Update(wsID, func(ws *store.Workspace) {
		if patch.Name != nil && strings.TrimSpace(*patch.Name) != "" {
			ws.Name = strings.TrimSpace(*patch.Name)
		}
		if patch.PermMode != nil {
			if strings.TrimSpace(*patch.PermMode) == "" {
				ws.PermMode = ""
			} else {
				ws.PermMode = core.NormalizePermMode(*patch.PermMode)
			}
		}
		if patch.SandboxMode != nil {
			if strings.TrimSpace(*patch.SandboxMode) == "" {
				ws.SandboxMode = ""
			} else {
				ws.SandboxMode = string(tools.NormalizeSandboxMode(*patch.SandboxMode))
			}
		}
		if patch.RoleModels != nil {
			ws.RoleModels = patch.RoleModels
		}
		if patch.TrustedHashes != nil {
			ws.TrustedHashes = patch.TrustedHashes
		}
	})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type workspaceFileEntry struct {
	RelPath string `json:"rel_path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size,omitempty"`
}

func (s *Server) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("wsID")
	ws, err := s.engine.Workspaces().Get(wsID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var vaultRoot string
	if s.cfg != nil {
		vaultRoot = s.cfg.Paths().Root
	}
	pg, err := tools.NewPathGuard(ws.RootPath, vaultRoot)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	const maxFiles = 400
	var files []workspaceFileEntry
	root := pg.WorkspaceRoot()
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, wErr error) error {
		if wErr != nil || path == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", "node_modules", ".worktrees", "dist", "vendor", ".superpowers":
				return filepath.SkipDir
			}
		}
		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		// Skip symlinks escaping workspace
		_, rel, inWS, rErr := pg.ResolveInWorkspace(path)
		if rErr != nil || !inWS || rel == "" || rel == "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		var sz int64
		if !d.IsDir() {
			if info, iErr := d.Info(); iErr == nil {
				sz = info.Size()
			}
		}
		files = append(files, workspaceFileEntry{
			RelPath: rel,
			IsDir:   d.IsDir(),
			Size:    sz,
		})
		return nil
	})
	if files == nil {
		files = []workspaceFileEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) handleReadWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("wsID")
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		writeJSONError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}
	ws, err := s.engine.Workspaces().Get(wsID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var vaultRoot string
	if s.cfg != nil {
		vaultRoot = s.cfg.Paths().Root
	}
	pg, err := tools.NewPathGuard(ws.RootPath, vaultRoot)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	canonAbs, cleanRel, inWS, err := pg.ResolveInWorkspace(rel)
	if err != nil || !inWS {
		writeJSONError(w, http.StatusForbidden, "path escapes workspace boundary")
		return
	}
	f, err := os.Open(canonAbs)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		writeJSONError(w, http.StatusBadRequest, "not a regular file")
		return
	}
	const maxPreviewRead = 256 * 1024
	raw, err := io.ReadAll(io.LimitReader(f, maxPreviewRead+1))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	truncated := false
	if len(raw) > maxPreviewRead {
		raw = raw[:maxPreviewRead]
		truncated = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rel_path":  cleanRel,
		"content":   config.EscapeBidi(string(raw)),
		"truncated": truncated,
		"size":      st.Size(),
	})
}

func (s *Server) handleListWorkspaceSkills(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("wsID")
	ws, err := s.engine.Workspaces().Get(wsID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var snap *config.Snapshot
	if s.cfg != nil {
		snap, _, _ = s.cfg.ForWorkspace(ws.RootPath, ws.TrustedHashes)
	}
	cat := tools.DiscoverSkills(snap, ws.RootPath)
	skills := cat.All()
	if skills == nil {
		skills = []tools.Skill{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

func (s *Server) handleListWorkspaceArtifacts(w http.ResponseWriter, r *http.Request) {
	wsID := r.PathValue("wsID")
	if _, err := s.engine.Workspaces().Get(wsID); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var items []map[string]any
	seenFiles := make(map[string]bool)
	if s.engine != nil && s.engine.Artifacts() != nil {
		if list, err := s.engine.Artifacts().List(wsID); err == nil {
			for _, art := range list {
				seenFiles[art.FileName] = true
				entry := map[string]any{
					"id":           art.ID,
					"name":         art.FileName,
					"title":        art.Title,
					"path":         art.Path,
					"kind":         art.Format,
					"format":       art.Format,
					"size_bytes":   art.SizeBytes,
					"session_id":   art.SessionID,
					"workspace_id": art.WorkspaceID,
					"created_at":   art.CreatedAt,
					"updated_at":   art.UpdatedAt,
				}
				if art.Format == "html" {
					if raw, rErr := os.ReadFile(art.Path); rErr == nil {
						pv := s.previews.Register(wsID, firstNonEmptyStr(art.Title, art.FileName), string(raw), art.Path)
						entry["preview_token"] = pv.Token
						entry["preview_url"] = pv.URL
					}
				}
				items = append(items, entry)
			}
		}
	}
	if s.cfg != nil {
		artDir := filepath.Join(s.cfg.Paths().WorkspacesDir(), wsID, "artifacts")
		if entries, err := os.ReadDir(artDir); err == nil {
			for _, ent := range entries {
				if ent.IsDir() || seenFiles[ent.Name()] {
					continue
				}
				ext := strings.ToLower(filepath.Ext(ent.Name()))
				if ext != ".html" && ext != ".md" {
					continue
				}
				full := filepath.Join(artDir, ent.Name())
				entry := map[string]any{
					"name": ent.Name(),
					"path": full,
					"kind": strings.TrimPrefix(ext, "."),
				}
				if ext == ".html" {
					if raw, rErr := os.ReadFile(full); rErr == nil {
						pv := s.previews.Register(wsID, ent.Name(), string(raw), full)
						entry["preview_token"] = pv.Token
						entry["preview_url"] = pv.URL
					}
				}
				items = append(items, entry)
			}
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"artifacts": items,
		"previews":  s.previews.List(wsID),
	})
}

// ============================================================================
// Sessions Endpoints (/api/sessions/*)
// ============================================================================

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	incArch := r.URL.Query().Get("include_archived") == "true"
	list := s.engine.Sessions().List(wsID, incArch)
	if list == nil {
		list = []*store.SessionMeta{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WorkspaceID   string `json:"workspace_id"`
		Title         string `json:"title"`
		AgentRole     string `json:"agent_role"`
		ModelOverride string `json:"model_override"`
		IsolateGit    bool   `json:"isolate_git"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ws, err := s.engine.Workspaces().Get(in.WorkspaceID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	meta, err := s.engine.Sessions().Create(ws.ID, in.Title, in.AgentRole)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var wtPath string
	if in.IsolateGit {
		if pool := s.worktreePool(); pool != nil && pool.IsGitRepo(r.Context(), ws.RootPath) {
			if lease, lErr := pool.Acquire(r.Context(), ws.RootPath, "sessions", meta.ID, ""); lErr == nil {
				wtPath = lease.Path
			}
		}
	}
	if strings.TrimSpace(in.ModelOverride) != "" || wtPath != "" {
		meta, _ = s.engine.Sessions().UpdateMeta(meta.ID, func(m *store.SessionMeta) {
			if strings.TrimSpace(in.ModelOverride) != "" {
				m.ModelOverride = strings.TrimSpace(in.ModelOverride)
			}
			if wtPath != "" {
				m.WorktreePath = wtPath
			}
		})
	}
	writeJSON(w, http.StatusCreated, meta)
}

func (s *Server) handleSearchSessions(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 20
	if rawLim := r.URL.Query().Get("limit"); rawLim != "" {
		if parsed, err := strconv.Atoi(rawLim); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	hits, err := s.engine.Sessions().Search(q, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	entries, err := s.engine.Sessions().LoadActiveBranch(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	snaps := s.readSessionSnapshotRecords(meta.WorkspaceID, id)
	writeJSON(w, http.StatusOK, map[string]any{
		"meta":      meta,
		"entries":   entries,
		"snapshots": snaps,
	})
}

func (s *Server) readSessionSnapshotRecords(wsID, sessionID string) []store.SnapshotRecord {
	if s.engine == nil || s.engine.Snapshots() == nil {
		return []store.SnapshotRecord{}
	}
	records, err := s.engine.Snapshots().ListSessionRecords(wsID, sessionID)
	if err != nil || records == nil {
		return []store.SnapshotRecord{}
	}
	return records
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var patch struct {
		Title         *string `json:"title"`
		AgentRole     *string `json:"agent_role"`
		ModelOverride *string `json:"model_override"`
		Draft         *string `json:"draft"`
		Pinned        *bool   `json:"pinned"`
		Archived      *bool   `json:"archived"`
	}
	if err := decodeJSONBody(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.engine.Sessions().UpdateMeta(id, func(m *store.SessionMeta) {
		if patch.Title != nil && strings.TrimSpace(*patch.Title) != "" {
			m.Title = strings.TrimSpace(*patch.Title)
		}
		if patch.AgentRole != nil && strings.TrimSpace(*patch.AgentRole) != "" {
			m.AgentRole = strings.TrimSpace(*patch.AgentRole)
		}
		if patch.ModelOverride != nil {
			m.ModelOverride = strings.TrimSpace(*patch.ModelOverride)
		}
		if patch.Draft != nil {
			m.Draft = *patch.Draft
		}
		if patch.Pinned != nil {
			m.Pinned = *patch.Pinned
		}
		if patch.Archived != nil {
			m.Archived = *patch.Archived
		}
	})
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var in struct {
		Force bool `json:"force"`
	}
	_ = decodeJSONBody(r, &in)
	force := in.Force || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("force")), "true")

	unlock, locked, err := store.TryFlock(s.engine.Sessions().RunLockPath(id))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !locked {
		writeJSONError(w, http.StatusConflict, core.ErrSessionBusy.Error())
		return
	}
	defer unlock()

	if curMeta, gErr := s.engine.Sessions().GetMeta(id); gErr == nil && curMeta != nil {
		meta = curMeta
	}
	if !force && s.isSessionWorktreeDirty(r.Context(), meta) {
		writeJSONError(w, http.StatusConflict, "session has unmerged changes in isolated git worktree; land changes or pass force=true")
		return
	}
	s.cleanupDeletedSessionWorktree(r.Context(), meta)

	if err := s.engine.Sessions().Delete(id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) cleanupDeletedSessionWorktree(ctx context.Context, meta *store.SessionMeta) {
	if meta == nil || s.cfg == nil {
		return
	}
	wtPath := strings.TrimSpace(meta.WorktreePath)
	if wtPath == "" {
		return
	}
	wtRoot := filepath.Clean(s.cfg.Paths().WorktreesDir())
	if resolvedRoot, err := filepath.EvalSymlinks(wtRoot); err == nil {
		wtRoot = filepath.Clean(resolvedRoot)
	}
	canonWT := filepath.Clean(wtPath)
	if resolvedWT, err := filepath.EvalSymlinks(wtPath); err == nil {
		canonWT = filepath.Clean(resolvedWT)
	}
	if !strings.HasPrefix(canonWT, wtRoot+string(filepath.Separator)) {
		return
	}

	var repoRoot string
	if s.engine != nil && meta.WorkspaceID != "" {
		if ws, err := s.engine.Workspaces().Get(meta.WorkspaceID); err == nil && ws != nil {
			repoRoot = ws.RootPath
		}
	}

	pool := s.worktreePool()
	if pool != nil {
		if rel, err := filepath.Rel(wtRoot, canonWT); err == nil {
			segs := strings.Split(filepath.ToSlash(rel), "/")
			if len(segs) == 2 && store.ValidateSafeID(segs[0]) == nil && store.ValidateSafeID(segs[1]) == nil {
				_ = pool.Release(ctx, &fleet.WorktreeLease{
					MissionID: segs[0],
					NodeID:    segs[1],
					RepoRoot:  repoRoot,
					Path:      canonWT,
					Branch:    fmt.Sprintf("gorness/fleet/%s/%s", segs[0], segs[1]),
				}, segs[0] == "sessions")
				return
			}
		}
	}
	_ = os.RemoveAll(canonWT)
}

func (s *Server) handleForkSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var in struct {
		AtSeq      *uint64 `json:"at_seq"`
		Title      string  `json:"title"`
		IsolateGit bool    `json:"isolate_git"`
	}
	_ = decodeJSONBody(r, &in)

	atSeq := meta.HeadSeq
	if in.AtSeq != nil {
		atSeq = *in.AtSeq
	}
	forked, err := s.engine.Sessions().Fork(id, atSeq, in.Title)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	wantWorktree := strings.TrimSpace(meta.WorktreePath) != "" || in.IsolateGit
	if wantWorktree {
		var wtPath string
		if pool := s.worktreePool(); pool != nil && s.engine != nil {
			if ws, wErr := s.engine.Workspaces().Get(forked.WorkspaceID); wErr == nil && ws != nil && pool.IsGitRepo(r.Context(), ws.RootPath) {
				baseBranch := ""
				if strings.TrimSpace(meta.WorktreePath) != "" {
					baseBranch = s.sessionWorktreeBranch(r.Context(), meta)
				}
				lease, lErr := pool.Acquire(r.Context(), ws.RootPath, "sessions", forked.ID, baseBranch)
				if lErr != nil && baseBranch != "" {
					lease, lErr = pool.Acquire(r.Context(), ws.RootPath, "sessions", forked.ID, "")
				}
				if lErr == nil && lease != nil {
					wtPath = lease.Path
				}
			}
		}
		forked, _ = s.engine.Sessions().UpdateMeta(forked.ID, func(m *store.SessionMeta) {
			m.WorktreePath = wtPath
		})
	}
	writeJSON(w, http.StatusCreated, forked)
}

func (s *Server) handleRewindSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.engine.Sessions().GetMeta(id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	var in struct {
		ToSeq        uint64 `json:"to_seq"`
		RestoreFiles bool   `json:"restore_files"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	unlock, locked, err := store.TryFlock(s.engine.Sessions().RunLockPath(id))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !locked {
		writeJSONError(w, http.StatusConflict, core.ErrSessionBusy.Error())
		return
	}
	defer unlock()

	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	wsRoot := ""
	if in.RestoreFiles {
		ws, wErr := s.engine.Workspaces().Get(meta.WorkspaceID)
		if wErr != nil {
			writeJSONError(w, http.StatusBadRequest, wErr.Error())
			return
		}
		wsRoot = ws.RootPath
		if strings.TrimSpace(meta.WorktreePath) != "" {
			wsRoot = strings.TrimSpace(meta.WorktreePath)
		}
	}

	updated, restored, err := s.engine.Sessions().Rewind(id, in.ToSeq, in.RestoreFiles, s.engine.Snapshots(), wsRoot)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if restored == nil {
		restored = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"meta":           updated,
		"restored_files": restored,
	})
}

func (s *Server) handleCompactSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.engine.CompactSession(r.Context(), id); err != nil {
		if errors.Is(err, core.ErrSessionBusy) {
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	meta, _ := s.engine.Sessions().GetMeta(id)
	entries, _ := s.engine.Sessions().LoadActiveBranch(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"meta":    meta,
		"entries": entries,
	})
}

func (s *Server) handleMoveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		TargetWorkspaceID string `json:"target_workspace_id"`
		Force             bool   `json:"force"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.engine.Workspaces().Get(in.TargetWorkspaceID); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	unlock, locked, err := store.TryFlock(s.engine.Sessions().RunLockPath(id))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !locked {
		writeJSONError(w, http.StatusConflict, core.ErrSessionBusy.Error())
		return
	}
	defer unlock()

	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	force := in.Force || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("force")), "true") || strings.TrimSpace(r.URL.Query().Get("force")) == "1"
	if !force && s.isSessionWorktreeDirty(r.Context(), meta) {
		writeJSONError(w, http.StatusConflict, "session has uncommitted or unmerged git worktree changes; land changes or pass force=true")
		return
	}
	if strings.TrimSpace(meta.WorktreePath) != "" && meta.WorkspaceID != in.TargetWorkspaceID {
		s.cleanupDeletedSessionWorktree(r.Context(), meta)
		_, _ = s.engine.Sessions().UpdateMeta(id, func(m *store.SessionMeta) {
			m.WorktreePath = ""
		})
	}

	moved, err := s.engine.Sessions().MoveWorkspace(id, in.TargetWorkspaceID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, moved)
}

func (s *Server) handleExportSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Format string `json:"format"`
		Title  string `json:"title"`
	}
	_ = decodeJSONBody(r, &in)
	format := strings.ToLower(firstNonEmptyStr(in.Format, r.URL.Query().Get("format")))
	if format == "" {
		format = "md"
	}
	meta, err := s.engine.Sessions().GetMeta(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	title := firstNonEmptyStr(in.Title, meta.Title, id)

	switch format {
	case "html":
		doc, err := s.engine.Sessions().ExportHTML(id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var savedPath string
		if s.engine != nil && s.engine.Artifacts() != nil {
			if saved, sErr := s.engine.Artifacts().Save(meta.WorkspaceID, store.Artifact{
				ID:        id,
				SessionID: id,
				Title:     title,
				Format:    "html",
				FileName:  id + ".html",
				Content:   doc,
			}); sErr == nil {
				savedPath = saved.Path
			}
		}
		if savedPath == "" && s.cfg != nil {
			savedPath = filepath.Join(s.cfg.Paths().WorkspacesDir(), meta.WorkspaceID, "artifacts", id+".html")
			_ = store.AtomicWriteFile(savedPath, []byte(doc), 0o600)
		}
		pv := s.previews.Register(meta.WorkspaceID, title, doc, savedPath)
		writeJSON(w, http.StatusOK, map[string]any{
			"format":        "html",
			"content":       doc,
			"artifact_path": savedPath,
			"preview_token": pv.Token,
			"preview_url":   pv.URL,
		})
	default:
		doc, err := s.engine.Sessions().ExportMarkdown(id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var savedPath string
		if s.engine != nil && s.engine.Artifacts() != nil {
			if saved, sErr := s.engine.Artifacts().Save(meta.WorkspaceID, store.Artifact{
				ID:        id,
				SessionID: id,
				Title:     title,
				Format:    "md",
				FileName:  id + ".md",
				Content:   doc,
			}); sErr == nil {
				savedPath = saved.Path
			}
		}
		if savedPath == "" && s.cfg != nil {
			savedPath = filepath.Join(s.cfg.Paths().WorkspacesDir(), meta.WorkspaceID, "artifacts", id+".md")
			_ = store.AtomicWriteFile(savedPath, []byte(doc), 0o600)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"format":        "md",
			"content":       doc,
			"artifact_path": savedPath,
		})
	}
}

// ============================================================================
// Runs & Approvals Endpoints (/api/runs/*, /api/approvals/*)
// ============================================================================

func (s *Server) handleListActiveRuns(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.engine.ActiveRuns())
}

func (s *Server) handleStartRunStream(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "engine unavailable")
		return
	}
	var req core.RunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	runID, evCh, err := s.engine.StartRun(s.bgCtx, req)
	if err != nil {
		if errors.Is(err, core.ErrSessionBusy) {
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("X-Gorness-Run-ID", runID)
	flusher, ok := prepareSSEHeaders(w)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.bgCtx.Done():
			return
		case ev, open := <-evCh:
			if !open {
				return
			}
			if err := writeSSEFrame(w, flusher, ev.Type, ev); err != nil {
				return
			}
		case t := <-ticker.C:
			if err := writeSSEFrame(w, flusher, "heartbeat", map[string]any{
				"type":      "heartbeat",
				"run_id":    runID,
				"timestamp": t.UTC(),
			}); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleSteerRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	var in struct {
		Message     string `json:"message"`
		Instruction string `json:"instruction"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	msg := firstNonEmptyStr(in.Message, in.Instruction)
	if msg == "" {
		writeJSONError(w, http.StatusBadRequest, "steer message is required")
		return
	}
	if err := s.engine.Steer(runID, msg); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run_id": runID})
}

func (s *Server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if err := s.engine.StopRun(runID); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run_id": runID})
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	list := s.engine.Broker().ListPending(wsID)
	if list == nil {
		list = []core.PendingApproval{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleResolveApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var dec core.ApprovalDecision
	if err := decodeJSONBody(r, &dec); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.engine.Broker().Resolve(id, dec); err != nil {
		if errors.Is(err, core.ErrApprovalNotSettled) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": err.Error(),
				"code":  "approval_not_settled",
			})
			return
		}
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// ============================================================================
// Always-On Endpoints (/api/alwayson/*)
// ============================================================================

func (s *Server) handleAlwaysOnState(w http.ResponseWriter, _ *http.Request) {
	var st alwayson.AlwaysOnState
	if s.alwaysOn != nil {
		st = s.alwaysOn.StateSnapshot()
	}
	if st.Inbox == nil {
		st.Inbox = []alwayson.InboxEvent{}
	}
	if st.Timers == nil {
		st.Timers = []alwayson.TimerEntry{}
	}
	if st.Deferred == nil {
		st.Deferred = []alwayson.DeferredCard{}
	}
	if st.Outbox == nil {
		st.Outbox = []alwayson.OutboxItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"inbox":            st.Inbox,
		"timers":           st.Timers,
		"deferred":         st.Deferred,
		"outbox":           st.Outbox,
		"updated_at":       st.UpdatedAt,
		"skill_candidates": s.listSkillCandidatesSnapshot(),
	})
}

func (s *Server) handleAlwaysOnWebhook(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	source := strings.TrimSpace(r.PathValue("source"))
	if source == "" {
		source = "local"
	}

	var body map[string]any
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body == nil {
		body = make(map[string]any)
	}

	wsID, _ := body["workspace_id"].(string)
	if strings.TrimSpace(wsID) == "" {
		wsID = strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	}
	if strings.TrimSpace(wsID) == "" && s.engine != nil {
		if list := s.engine.Workspaces().List(); len(list) > 0 {
			wsID = list[0].ID
		}
	}
	if strings.TrimSpace(wsID) == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	sourceID, _ := body["source_id"].(string)
	sessionID, _ := body["session_id"].(string)
	agentRole, _ := body["agent_role"].(string)
	inboxKey, _ := body["inbox_key"].(string)

	payload := body
	if nested, ok := body["payload"].(map[string]any); ok && len(nested) > 0 {
		payload = nested
	}

	deduped, err := s.alwaysOn.Enqueue(alwayson.InboxEvent{
		InboxKey:    inboxKey,
		Source:      "webhook:" + source,
		SourceID:    sourceID,
		WorkspaceID: wsID,
		SessionID:   sessionID,
		AgentRole:   agentRole,
		Payload:     payload,
	})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"source":  source,
		"deduped": deduped,
	})
}

func (s *Server) handleListTimers(w http.ResponseWriter, _ *http.Request) {
	if s.alwaysOn == nil {
		writeJSON(w, http.StatusOK, []alwayson.TimerEntry{})
		return
	}
	list, err := s.alwaysOn.ListTimers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []alwayson.TimerEntry{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateTimer(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	var in struct {
		ID           string `json:"id"`
		WorkspaceID  string `json:"workspace_id"`
		SessionID    string `json:"session_id"`
		AgentRole    string `json:"agent_role"`
		Kind         string `json:"kind"`
		CronExpr     string `json:"cron_expr"`
		AfterSeconds int    `json:"after_seconds"`
		Note         string `json:"note"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if strings.EqualFold(in.Kind, alwayson.TimerKindCron) || strings.TrimSpace(in.CronExpr) != "" {
		entry := alwayson.TimerEntry{
			ID:          in.ID,
			WorkspaceID: in.WorkspaceID,
			SessionID:   in.SessionID,
			AgentRole:   in.AgentRole,
			Kind:        alwayson.TimerKindCron,
			CronExpr:    in.CronExpr,
			Note:        in.Note,
			Enabled:     enabled,
		}
		if err := s.alwaysOn.UpsertCron(entry); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, entry)
		return
	}

	delay := time.Duration(in.AfterSeconds) * time.Second
	if delay <= 0 {
		delay = 60 * time.Second
	}
	entry, err := s.alwaysOn.ScheduleFollowUp(in.WorkspaceID, in.SessionID, delay, in.Note)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) handleTickTimers(w http.ResponseWriter, _ *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	fired, err := s.alwaysOn.TickTimers(s.now())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fired": fired})
}

func (s *Server) handleListDeferred(w http.ResponseWriter, _ *http.Request) {
	if s.alwaysOn == nil {
		writeJSON(w, http.StatusOK, []alwayson.DeferredCard{})
		return
	}
	cards, err := s.alwaysOn.ListDeferred()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cards == nil {
		cards = []alwayson.DeferredCard{}
	}
	writeJSON(w, http.StatusOK, cards)
}

func (s *Server) handleResolveDeferred(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	id := r.PathValue("id")
	var in struct {
		Decision        string   `json:"decision"`
		Approved        bool     `json:"approved"`
		SelectedHunkIDs []string `json:"selected_hunk_ids"`
		Remember        bool     `json:"remember"`
		Feedback        string   `json:"feedback"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.alwaysOn.ResolveDeferredCard(id, alwayson.DeferredResolution{
		Decision:        in.Decision,
		Approved:        in.Approved,
		SelectedHunkIDs: in.SelectedHunkIDs,
		Remember:        in.Remember,
		Feedback:        in.Feedback,
	}); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleListOutbox(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSON(w, http.StatusOK, []alwayson.OutboxItem{})
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := s.alwaysOn.ListOutbox(limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []alwayson.OutboxItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleRunDream(w http.ResponseWriter, r *http.Request) {
	if s.alwaysOn == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "alwayson service unavailable")
		return
	}
	var in struct {
		WorkspaceID string `json:"workspace_id"`
	}
	_ = decodeJSONBody(r, &in)
	paths, err := s.alwaysOn.RunDreamPass(r.Context(), in.WorkspaceID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidate_skills": paths})
}

// ============================================================================
// Fleet Visual Multi-Agent Orchestration Endpoints (/api/fleet/*)
// ============================================================================

func (s *Server) handleListMissions(w http.ResponseWriter, r *http.Request) {
	if s.fleet == nil {
		writeJSON(w, http.StatusOK, []*fleet.MissionSnapshot{})
		return
	}
	wsID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if wsID != "" {
		list, err := s.fleet.ListMissions(wsID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if list == nil {
			list = []*fleet.MissionSnapshot{}
		}
		writeJSON(w, http.StatusOK, list)
		return
	}

	var all []*fleet.MissionSnapshot
	for _, ws := range s.engine.Workspaces().List() {
		if list, err := s.fleet.ListMissions(ws.ID); err == nil {
			all = append(all, list...)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})
	if all == nil {
		all = []*fleet.MissionSnapshot{}
	}
	writeJSON(w, http.StatusOK, all)
}

func (s *Server) handleStartMission(w http.ResponseWriter, r *http.Request) {
	if s.fleet == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "fleet manager unavailable")
		return
	}
	var req fleet.MissionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	snap, _, err := s.fleet.StartMission(s.bgCtx, req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

func (s *Server) resolveMissionWorkspace(wsIDHint, missionID string) (string, error) {
	if trimmed := strings.TrimSpace(wsIDHint); trimmed != "" {
		return trimmed, nil
	}
	if s.fleet == nil || s.engine == nil {
		return "", fleet.ErrMissionNotFound
	}
	for _, ws := range s.engine.Workspaces().List() {
		if _, err := s.fleet.GetMission(ws.ID, missionID); err == nil {
			return ws.ID, nil
		}
	}
	return "", fmt.Errorf("%w: %s", fleet.ErrMissionNotFound, missionID)
}

func (s *Server) handleGetMission(w http.ResponseWriter, r *http.Request) {
	missionID := r.PathValue("id")
	wsID, err := s.resolveMissionWorkspace(r.URL.Query().Get("workspace_id"), missionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	snap, err := s.fleet.GetMission(wsID, missionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleStreamMission(w http.ResponseWriter, r *http.Request) {
	if s.fleet == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "fleet manager unavailable")
		return
	}
	missionID := r.PathValue("id")
	wsID, err := s.resolveMissionWorkspace(r.URL.Query().Get("workspace_id"), missionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	snap, ch, unsub, err := s.fleet.Subscribe(wsID, missionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	defer unsub()

	flusher, ok := prepareSSEHeaders(w)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	initEv := fleet.MissionEvent{
		Seq:         0,
		Type:        fleet.EventSnapshotSync,
		MissionID:   snap.ID,
		WorkspaceID: snap.WorkspaceID,
		Snapshot:    snap,
		Timestamp:   s.now().UTC(),
	}
	if err := writeSSEFrame(w, flusher, initEv.Type, initEv); err != nil {
		return
	}
	if snap.Status == fleet.MissionCompleted || snap.Status == fleet.MissionFailed || snap.Status == fleet.MissionCancelled {
		termType := fleet.EventMissionCompleted
		switch snap.Status {
		case fleet.MissionFailed:
			termType = fleet.EventMissionFailed
		case fleet.MissionCancelled:
			termType = fleet.EventMissionCancelled
		}
		_ = writeSSEFrame(w, flusher, termType, fleet.MissionEvent{
			Type:        termType,
			MissionID:   snap.ID,
			WorkspaceID: snap.WorkspaceID,
			Snapshot:    snap,
			Timestamp:   s.now().UTC(),
		})
		return
	}

	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.bgCtx.Done():
			return
		case ev, open := <-ch:
			if !open {
				if finSnap, gErr := s.fleet.GetMission(wsID, missionID); gErr == nil {
					termType := fleet.EventMissionCompleted
					switch finSnap.Status {
					case fleet.MissionFailed:
						termType = fleet.EventMissionFailed
					case fleet.MissionCancelled:
						termType = fleet.EventMissionCancelled
					}
					_ = writeSSEFrame(w, flusher, termType, fleet.MissionEvent{
						Type:        termType,
						MissionID:   finSnap.ID,
						WorkspaceID: finSnap.WorkspaceID,
						Snapshot:    finSnap,
						Timestamp:   s.now().UTC(),
					})
				}
				return
			}
			if err := writeSSEFrame(w, flusher, ev.Type, ev); err != nil {
				return
			}
		case t := <-ticker.C:
			if err := writeSSEFrame(w, flusher, "heartbeat", map[string]any{
				"type":       "heartbeat",
				"mission_id": missionID,
				"timestamp":  t.UTC(),
			}); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleInspectNodeDiff(w http.ResponseWriter, r *http.Request) {
	missionID := r.PathValue("id")
	nodeID := r.PathValue("nodeID")
	wsID, err := s.resolveMissionWorkspace(r.URL.Query().Get("workspace_id"), missionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	diff, err := s.fleet.InspectNodeDiff(wsID, missionID, nodeID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mission_id": missionID,
		"node_id":    nodeID,
		"diff":       config.EscapeBidi(diff),
	})
}

func (s *Server) handleSteerNode(w http.ResponseWriter, r *http.Request) {
	missionID := r.PathValue("id")
	nodeID := r.PathValue("nodeID")
	var in struct {
		WorkspaceID string `json:"workspace_id"`
		Instruction string `json:"instruction"`
		Message     string `json:"message"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	wsID, err := s.resolveMissionWorkspace(firstNonEmptyStr(in.WorkspaceID, r.URL.Query().Get("workspace_id")), missionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	msg := firstNonEmptyStr(in.Instruction, in.Message)
	if msg == "" {
		writeJSONError(w, http.StatusBadRequest, "steer instruction is required")
		return
	}
	if err := s.fleet.SteerNode(wsID, missionID, nodeID, msg); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mission_id": missionID, "node_id": nodeID})
}

func (s *Server) handleStopMission(w http.ResponseWriter, r *http.Request) {
	missionID := r.PathValue("id")
	wsID, err := s.resolveMissionWorkspace(r.URL.Query().Get("workspace_id"), missionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.fleet.StopMission(wsID, missionID); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mission_id": missionID})
}

// ============================================================================
// Safe External Copy-Open & HTML Artifact Previews
// ============================================================================

func (s *Server) handleOpenSafe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WorkspaceID string `json:"workspace_id"`
		Path        string `json:"path"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var wsRoot string
	if strings.TrimSpace(in.WorkspaceID) != "" && s.engine != nil {
		ws, err := s.engine.Workspaces().Get(in.WorkspaceID)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		wsRoot = ws.RootPath
	}

	res, err := s.opener.Open(r.Context(), wsRoot, in.Path)
	if err != nil {
		if errors.Is(err, ErrForbiddenOpenPath) || errors.Is(err, ErrTOCTOUDetected) {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListPreviews(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	writeJSON(w, http.StatusOK, s.previews.List(wsID))
}

func (s *Server) handleCreatePreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WorkspaceID string `json:"workspace_id"`
		Title       string `json:"title"`
		HTML        string `json:"html"`
		SaveName    string `json:"save_name"`
	}
	if err := decodeJSONBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.HTML) == "" {
		writeJSONError(w, http.StatusBadRequest, "html content is required")
		return
	}

	var savedPath string
	if strings.TrimSpace(in.WorkspaceID) != "" && strings.TrimSpace(in.SaveName) != "" {
		if _, err := s.engine.Workspaces().Get(in.WorkspaceID); err == nil {
			safeStem := sanitizeCopyStem(strings.TrimSuffix(in.SaveName, ".html"))
			if s.engine != nil && s.engine.Artifacts() != nil {
				if saved, sErr := s.engine.Artifacts().Save(in.WorkspaceID, store.Artifact{
					ID:       safeStem,
					Title:    firstNonEmptyStr(in.Title, safeStem),
					Format:   "html",
					FileName: safeStem + ".html",
					Content:  in.HTML,
				}); sErr == nil {
					savedPath = saved.Path
				}
			}
			if savedPath == "" && s.cfg != nil {
				savedPath = filepath.Join(s.cfg.Paths().WorkspacesDir(), in.WorkspaceID, "artifacts", safeStem+".html")
				_ = store.AtomicWriteFile(savedPath, []byte(in.HTML), 0o600)
			}
		}
	}

	item := s.previews.Register(in.WorkspaceID, in.Title, in.HTML, savedPath)
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleServePreview(w http.ResponseWriter, r *http.Request) {
	s.previews.ServePreview(w, r.PathValue("token"))
}

// ============================================================================
// Wire Helpers & Secret Sanitization
// ============================================================================

func sanitizeConfigSnapshot(snap *config.Snapshot) *config.Snapshot {
	if snap == nil {
		return nil
	}
	cp := snap.Clone()
	for id, p := range cp.Providers {
		ref := strings.TrimSpace(p.APIKeyRef)
		if ref != "" && !strings.HasPrefix(ref, "env:") && !strings.HasPrefix(ref, "keychain:") && !strings.HasPrefix(ref, "cmd:") {
			p.APIKeyRef = "[REDACTED]"
		}
		if len(p.Headers) > 0 {
			p.Headers = config.SanitizeSecretMap(p.Headers)
		}
		cp.Providers[id] = p
	}
	for id, a := range cp.Agents {
		ref := strings.TrimSpace(a.APIKeyRef)
		if ref != "" && !strings.HasPrefix(ref, "env:") && !strings.HasPrefix(ref, "keychain:") && !strings.HasPrefix(ref, "cmd:") {
			a.APIKeyRef = "[REDACTED]"
			cp.Agents[id] = a
		}
	}
	for id, srv := range cp.MCP.Servers {
		if len(srv.Env) > 0 {
			srv.Env = config.SanitizeSecretMap(srv.Env)
		}
		if len(srv.Headers) > 0 {
			srv.Headers = config.SanitizeSecretMap(srv.Headers)
		}
		cp.MCP.Servers[id] = srv
	}
	return cp
}

func prepareSSEHeaders(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return flusher, true
}

func writeSSEFrame(w io.Writer, flusher http.Flusher, eventType string, payload any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return err
	}
	line := bytes.TrimRight(buf.Bytes(), "\n")
	evName := strings.TrimSpace(eventType)
	if evName == "" {
		evName = "message"
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evName, string(line)); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		http.Error(w, `{"error":"json encode failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{
		"error": config.EscapeBidi(msg),
	})
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
