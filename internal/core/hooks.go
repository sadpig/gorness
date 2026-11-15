package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorness.dev/gorness/internal/tools"
)

var (
	ErrEmptyPluginID           = errors.New("plugin id is required")
	ErrPluginAlreadyMounted    = errors.New("plugin is already mounted")
	ErrPluginNotMounted        = errors.New("plugin is not mounted")
	ErrMissingPluginDependency = errors.New("missing required plugin dependency")
)

type PreStepEvent struct {
	RunID            string
	WorkspaceID      string
	SessionID        string
	AgentRole        string
	Step             int
	DynamicReminders []string
	Metadata         map[string]any
}

type PreToolUseEvent struct {
	RunID       string
	WorkspaceID string
	SessionID   string
	AgentRole   string
	Step        int
	Call        ToolInvocation
}

type PostToolUseEvent struct {
	RunID       string
	WorkspaceID string
	SessionID   string
	AgentRole   string
	Step        int
	Call        ToolInvocation
	Result      *tools.ToolResult
	Duration    time.Duration
	Err         error
}

type TurnStoppingEvent struct {
	RunID          string
	WorkspaceID    string
	SessionID      string
	AgentRole      string
	Step           int
	StopReason     string
	FollowUpPrompt string
	VetoStop       bool
}

type (
	PreStepHook      func(ctx context.Context, ev *PreStepEvent) error
	PreToolUseHook   func(ctx context.Context, ev *PreToolUseEvent) (GateResult, error)
	PostToolUseHook  func(ctx context.Context, ev *PostToolUseEvent) error
	TurnStoppingHook func(ctx context.Context, ev *TurnStoppingEvent) error
)

type Plugin struct {
	ID        string
	DependsOn []string
	Setup     func(sc *Scope) error
}

// Scope 维护插件或子系统的层级生命周期栈，关闭时严格按 LIFO（后进先出）逆序执行清理回调。
type Scope struct {
	id     string
	bus    *HookBus
	reg    *tools.Registry
	parent *Scope

	mu       sync.Mutex
	closed   bool
	children []*Scope
	cleanups []func() error
}

func NewScope(id string) *Scope {
	return &Scope{id: strings.TrimSpace(id)}
}

func (s *Scope) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *Scope) Child(id string) *Scope {
	child := &Scope{
		id:     strings.TrimSpace(id),
		bus:    s.bus,
		reg:    s.reg,
		parent: s,
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = child.Close()
		return child
	}
	s.children = append(s.children, child)
	s.mu.Unlock()
	return child
}

func (s *Scope) Defer(fn func() error) {
	if s == nil || fn == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = fn()
		return
	}
	s.cleanups = append(s.cleanups, fn)
	s.mu.Unlock()
}

func (s *Scope) OnCleanup(fn func()) {
	if fn == nil {
		return
	}
	s.Defer(func() error {
		fn()
		return nil
	})
}

func (s *Scope) RegisterTool(t tools.Tool) {
	if s == nil || strings.TrimSpace(t.Name) == "" {
		return
	}
	toolName := strings.TrimSpace(t.Name)
	if s.reg != nil {
		s.reg.Register(t)
	}
	if s.bus != nil {
		s.bus.recordPluginTool(s.id, t)
	}
	s.OnCleanup(func() {
		if s.bus != nil {
			s.bus.removePluginTool(s.id, toolName)
		}
		if s.reg != nil {
			s.reg.Unregister(toolName)
		}
	})
}

func (s *Scope) OnPreStep(h PreStepHook) {
	if s == nil || s.bus == nil || h == nil {
		return
	}
	unsub := s.bus.addPreStep(s.id, h)
	s.OnCleanup(unsub)
}

func (s *Scope) OnPreToolUse(h PreToolUseHook) {
	if s == nil || s.bus == nil || h == nil {
		return
	}
	unsub := s.bus.addPreToolUse(s.id, h)
	s.OnCleanup(unsub)
}

func (s *Scope) OnPostToolUse(h PostToolUseHook) {
	if s == nil || s.bus == nil || h == nil {
		return
	}
	unsub := s.bus.addPostToolUse(s.id, h)
	s.OnCleanup(unsub)
}

func (s *Scope) OnTurnStopping(h TurnStoppingHook) {
	if s == nil || s.bus == nil || h == nil {
		return
	}
	unsub := s.bus.addTurnStopping(s.id, h)
	s.OnCleanup(unsub)
}

func (s *Scope) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	children := append([]*Scope(nil), s.children...)
	cleanups := append([]func() error(nil), s.cleanups...)
	s.children = nil
	s.cleanups = nil
	s.mu.Unlock()

	var firstErr error
	// 1. 先按逆序卸载所有子 Scope
	for i := len(children) - 1; i >= 0; i-- {
		if err := children[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// 2. 再按 LIFO 逆序执行当前 Scope 注册的清理函数
	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := cleanups[i](); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type hookEntry[H any] struct {
	id       uint64
	pluginID string
	fn       H
}

type mountedPlugin struct {
	plugin     Plugin
	scope      *Scope
	mountOrder uint64
}

type compiledHooks struct {
	preStep      []PreStepHook
	preToolUse   []PreToolUseHook
	postToolUse  []PostToolUseHook
	turnStopping []TurnStoppingHook
}

// HookBus 是微内核插件与四阶段生命周期钩子总线：
// Freeze() 将已挂载的钩子预编译为不可变切片存入 atomic.Pointer，使热路径分发达到零互斥锁、零堆分配开销；
// UnmountPlugin 按拓扑依赖逆序（兼 LIFO 挂载逆序）先清理依赖方再清理被依赖插件，并原子刷新冻结快照。
type HookBus struct {
	mu           sync.RWMutex
	reg          *tools.Registry
	nextHookID   uint64
	nextMountSeq uint64
	frozen       bool

	plugins      map[string]*mountedPlugin
	pluginTools  map[string]tools.Tool // key: pluginID + "\x00" + toolName
	preStep      []hookEntry[PreStepHook]
	preToolUse   []hookEntry[PreToolUseHook]
	postToolUse  []hookEntry[PostToolUseHook]
	turnStopping []hookEntry[TurnStoppingHook]

	compiled atomic.Pointer[compiledHooks]
}

func NewHookBus(reg *tools.Registry) *HookBus {
	return &HookBus{
		reg:         reg,
		plugins:     make(map[string]*mountedPlugin),
		pluginTools: make(map[string]tools.Tool),
	}
}

func (b *HookBus) Registry() *tools.Registry {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.reg
}

func (b *HookBus) PluginTools() []tools.Tool {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	out := make([]tools.Tool, 0, len(b.pluginTools))
	for _, t := range b.pluginTools {
		out = append(out, t)
	}
	b.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (b *HookBus) recordPluginTool(pluginID string, t tools.Tool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.pluginTools[pluginID+"\x00"+strings.TrimSpace(t.Name)] = t
	b.mu.Unlock()
}

func (b *HookBus) removePluginTool(pluginID, toolName string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.pluginTools, pluginID+"\x00"+strings.TrimSpace(toolName))
	b.mu.Unlock()
}

func (b *HookBus) MountPlugin(p Plugin) error {
	if b == nil {
		return errors.New("nil HookBus")
	}
	id := strings.TrimSpace(p.ID)
	if id == "" {
		return ErrEmptyPluginID
	}
	p.ID = id

	b.mu.Lock()
	if _, exists := b.plugins[id]; exists {
		b.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginAlreadyMounted, id)
	}
	deps := make([]string, 0, len(p.DependsOn))
	for _, rawDep := range p.DependsOn {
		dep := strings.TrimSpace(rawDep)
		if dep == "" {
			continue
		}
		if dep == id {
			b.mu.Unlock()
			return fmt.Errorf("%w: plugin %q cannot depend on itself", ErrMissingPluginDependency, id)
		}
		if _, ok := b.plugins[dep]; !ok {
			b.mu.Unlock()
			return fmt.Errorf("%w: plugin %q requires %q", ErrMissingPluginDependency, id, dep)
		}
		deps = append(deps, dep)
	}
	p.DependsOn = deps
	reg := b.reg
	b.mu.Unlock()

	sc := &Scope{
		id:  id,
		bus: b,
		reg: reg,
	}

	if p.Setup != nil {
		if err := p.Setup(sc); err != nil {
			_ = sc.Close()
			return fmt.Errorf("mount plugin %q: %w", id, err)
		}
	}

	b.mu.Lock()
	// 二次复核并发挂载冲突与依赖存活状态
	if _, exists := b.plugins[id]; exists {
		b.mu.Unlock()
		_ = sc.Close()
		return fmt.Errorf("%w: %s", ErrPluginAlreadyMounted, id)
	}
	for _, dep := range p.DependsOn {
		if _, ok := b.plugins[dep]; !ok {
			b.mu.Unlock()
			_ = sc.Close()
			return fmt.Errorf("%w: plugin %q requires %q", ErrMissingPluginDependency, id, dep)
		}
	}
	b.nextMountSeq++
	b.plugins[id] = &mountedPlugin{
		plugin:     p,
		scope:      sc,
		mountOrder: b.nextMountSeq,
	}
	if b.frozen {
		b.recompileLocked()
	}
	b.mu.Unlock()

	return nil
}

// UnmountPlugin 查找目标插件及其全部传递依赖方，按挂载逆序（由于挂载强制依赖先于被依赖方，挂载逆序天然满足拓扑逆序与 LIFO）逐一卸载。
func (b *HookBus) UnmountPlugin(pluginID string) error {
	if b == nil {
		return nil
	}
	target := strings.TrimSpace(pluginID)
	if target == "" {
		return ErrEmptyPluginID
	}

	b.mu.Lock()
	if _, ok := b.plugins[target]; !ok {
		b.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotMounted, target)
	}

	// BFS 收集所有直接或间接依赖 target 的已挂载插件
	toUnmount := map[string]*mountedPlugin{
		target: b.plugins[target],
	}
	changed := true
	for changed {
		changed = false
		for candID, mp := range b.plugins {
			if _, already := toUnmount[candID]; already {
				continue
			}
			for _, dep := range mp.plugin.DependsOn {
				if _, hit := toUnmount[dep]; hit {
					toUnmount[candID] = mp
					changed = true
					break
				}
			}
		}
	}

	ordered := make([]*mountedPlugin, 0, len(toUnmount))
	for _, mp := range toUnmount {
		ordered = append(ordered, mp)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].mountOrder > ordered[j].mountOrder
	})

	for _, mp := range ordered {
		delete(b.plugins, mp.plugin.ID)
	}
	b.mu.Unlock()

	var firstErr error
	for _, mp := range ordered {
		if err := mp.scope.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	b.mu.Lock()
	if b.frozen {
		b.recompileLocked()
	}
	b.mu.Unlock()

	return firstErr
}

func (b *HookBus) MountedPlugins() []string {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	list := make([]*mountedPlugin, 0, len(b.plugins))
	for _, mp := range b.plugins {
		list = append(list, mp)
	}
	b.mu.RUnlock()

	sort.Slice(list, func(i, j int) bool {
		return list[i].mountOrder < list[j].mountOrder
	})
	out := make([]string, len(list))
	for i, mp := range list {
		out[i] = mp.plugin.ID
	}
	return out
}

func (b *HookBus) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	ordered := make([]*mountedPlugin, 0, len(b.plugins))
	for _, mp := range b.plugins {
		ordered = append(ordered, mp)
	}
	b.plugins = make(map[string]*mountedPlugin)
	b.mu.Unlock()

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].mountOrder > ordered[j].mountOrder
	})

	var firstErr error
	for _, mp := range ordered {
		if err := mp.scope.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	b.mu.Lock()
	b.preStep = nil
	b.preToolUse = nil
	b.postToolUse = nil
	b.turnStopping = nil
	b.pluginTools = make(map[string]tools.Tool)
	if b.frozen {
		b.recompileLocked()
	}
	b.mu.Unlock()
	return firstErr
}

func (b *HookBus) Freeze() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.frozen = true
	b.recompileLocked()
	b.mu.Unlock()
}

func (b *HookBus) IsFrozen() bool {
	if b == nil {
		return false
	}
	return b.compiled.Load() != nil
}

func (b *HookBus) recompileLocked() {
	snap := &compiledHooks{}
	if len(b.preStep) > 0 {
		snap.preStep = make([]PreStepHook, len(b.preStep))
		for i, e := range b.preStep {
			snap.preStep[i] = e.fn
		}
	}
	if len(b.preToolUse) > 0 {
		snap.preToolUse = make([]PreToolUseHook, len(b.preToolUse))
		for i, e := range b.preToolUse {
			snap.preToolUse[i] = e.fn
		}
	}
	if len(b.postToolUse) > 0 {
		snap.postToolUse = make([]PostToolUseHook, len(b.postToolUse))
		for i, e := range b.postToolUse {
			snap.postToolUse[i] = e.fn
		}
	}
	if len(b.turnStopping) > 0 {
		snap.turnStopping = make([]TurnStoppingHook, len(b.turnStopping))
		for i, e := range b.turnStopping {
			snap.turnStopping[i] = e.fn
		}
	}
	b.compiled.Store(snap)
}

func (b *HookBus) OnPreStep(h PreStepHook) func() {
	return b.addPreStep("", h)
}

func (b *HookBus) OnPreToolUse(h PreToolUseHook) func() {
	return b.addPreToolUse("", h)
}

func (b *HookBus) OnPostToolUse(h PostToolUseHook) func() {
	return b.addPostToolUse("", h)
}

func (b *HookBus) OnTurnStopping(h TurnStoppingHook) func() {
	return b.addTurnStopping("", h)
}

func (b *HookBus) addPreStep(pluginID string, h PreStepHook) func() {
	if b == nil || h == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextHookID++
	id := b.nextHookID
	b.preStep = append(b.preStep, hookEntry[PreStepHook]{id: id, pluginID: pluginID, fn: h})
	if b.frozen {
		b.recompileLocked()
	}
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.preStep = removeHookByID(b.preStep, id)
			if b.frozen {
				b.recompileLocked()
			}
			b.mu.Unlock()
		})
	}
}

func (b *HookBus) addPreToolUse(pluginID string, h PreToolUseHook) func() {
	if b == nil || h == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextHookID++
	id := b.nextHookID
	b.preToolUse = append(b.preToolUse, hookEntry[PreToolUseHook]{id: id, pluginID: pluginID, fn: h})
	if b.frozen {
		b.recompileLocked()
	}
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.preToolUse = removeHookByID(b.preToolUse, id)
			if b.frozen {
				b.recompileLocked()
			}
			b.mu.Unlock()
		})
	}
}

func (b *HookBus) addPostToolUse(pluginID string, h PostToolUseHook) func() {
	if b == nil || h == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextHookID++
	id := b.nextHookID
	b.postToolUse = append(b.postToolUse, hookEntry[PostToolUseHook]{id: id, pluginID: pluginID, fn: h})
	if b.frozen {
		b.recompileLocked()
	}
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.postToolUse = removeHookByID(b.postToolUse, id)
			if b.frozen {
				b.recompileLocked()
			}
			b.mu.Unlock()
		})
	}
}

func (b *HookBus) addTurnStopping(pluginID string, h TurnStoppingHook) func() {
	if b == nil || h == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextHookID++
	id := b.nextHookID
	b.turnStopping = append(b.turnStopping, hookEntry[TurnStoppingHook]{id: id, pluginID: pluginID, fn: h})
	if b.frozen {
		b.recompileLocked()
	}
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.turnStopping = removeHookByID(b.turnStopping, id)
			if b.frozen {
				b.recompileLocked()
			}
			b.mu.Unlock()
		})
	}
}

func removeHookByID[H any](in []hookEntry[H], id uint64) []hookEntry[H] {
	for i := range in {
		if in[i].id == id {
			out := make([]hookEntry[H], 0, len(in)-1)
			out = append(out, in[:i]...)
			out = append(out, in[i+1:]...)
			return out
		}
	}
	return in
}

func (b *HookBus) RunPreStep(ctx context.Context, ev *PreStepEvent) error {
	if b == nil {
		return nil
	}
	if snap := b.compiled.Load(); snap != nil {
		for i := 0; i < len(snap.preStep); i++ {
			if err := snap.preStep[i](ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}

	b.mu.RLock()
	handlers := make([]PreStepHook, len(b.preStep))
	for i, e := range b.preStep {
		handlers[i] = e.fn
	}
	b.mu.RUnlock()

	for _, fn := range handlers {
		if err := fn(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func (b *HookBus) RunPreToolUse(ctx context.Context, ev *PreToolUseEvent) (GateResult, error) {
	if b == nil {
		return GateResult{}, nil
	}
	if snap := b.compiled.Load(); snap != nil {
		return runPreToolSlice(ctx, snap.preToolUse, ev)
	}

	b.mu.RLock()
	handlers := make([]PreToolUseHook, len(b.preToolUse))
	for i, e := range b.preToolUse {
		handlers[i] = e.fn
	}
	b.mu.RUnlock()

	return runPreToolSlice(ctx, handlers, ev)
}

// runPreToolSlice 依次执行所有 PreToolUse 钩子：
// 若任一钩子返回 GateDeny 则立即熔断拒绝；若多个钩子返回动作，按 Deny > Defer > Ask 严格收敛，绝不允许钩子用 Allow 跳过后续门控步骤。
func runPreToolSlice(ctx context.Context, handlers []PreToolUseHook, ev *PreToolUseEvent) (GateResult, error) {
	var chosen GateResult
	for i := 0; i < len(handlers); i++ {
		res, err := handlers[i](ctx, ev)
		if err != nil {
			return GateResult{
				Action:     GateDeny,
				ReasonCode: "hook-error",
				Reason:     err.Error(),
			}, err
		}
		switch res.Action {
		case GateDeny:
			if res.ReasonCode == "" {
				res.ReasonCode = "hook-deny"
			}
			return res, nil
		case GateDefer:
			if chosen.Action != GateDefer {
				if res.ReasonCode == "" {
					res.ReasonCode = "hook-defer"
				}
				chosen = res
			}
		case GateAsk:
			if !chosen.RequiresAction() {
				if res.ReasonCode == "" {
					res.ReasonCode = "hook-ask"
				}
				chosen = res
			}
		}
	}
	return chosen, nil
}

func (b *HookBus) RunPostToolUse(ctx context.Context, ev *PostToolUseEvent) error {
	if b == nil {
		return nil
	}
	if snap := b.compiled.Load(); snap != nil {
		for i := 0; i < len(snap.postToolUse); i++ {
			if err := snap.postToolUse[i](ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}

	b.mu.RLock()
	handlers := make([]PostToolUseHook, len(b.postToolUse))
	for i, e := range b.postToolUse {
		handlers[i] = e.fn
	}
	b.mu.RUnlock()

	for _, fn := range handlers {
		if err := fn(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func (b *HookBus) RunTurnStopping(ctx context.Context, ev *TurnStoppingEvent) error {
	if b == nil {
		return nil
	}
	if snap := b.compiled.Load(); snap != nil {
		for i := 0; i < len(snap.turnStopping); i++ {
			if err := snap.turnStopping[i](ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}

	b.mu.RLock()
	handlers := make([]TurnStoppingHook, len(b.turnStopping))
	for i, e := range b.turnStopping {
		handlers[i] = e.fn
	}
	b.mu.RUnlock()

	for _, fn := range handlers {
		if err := fn(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}
