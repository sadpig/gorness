package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchDebounce = 80 * time.Millisecond

// Watch 挂载 fsnotify 监听 Root、AgentsDir、WorkflowsDir 与 SkillsDir（80ms 防抖）；
// 若重载遇到语法错误，严格保留上一份合法 Snapshot 并向回调推送结构化 *ReloadDiagnostic。
func (m *Manager) Watch(ctx context.Context, onChange func(*Snapshot, *ReloadDiagnostic)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	watchDirs := []string{
		m.paths.Root,
		m.paths.AgentsDir(),
		m.paths.WorkflowsDir(),
		m.paths.SkillsDir(),
	}
	for _, dir := range watchDirs {
		if err := watcher.Add(dir); err != nil {
			_ = watcher.Close()
			return err
		}
	}
	// 同时监听已存在的技能一级子目录 (<skill>/SKILL.md)。
	if entries, err := os.ReadDir(m.paths.SkillsDir()); err == nil {
		for _, ent := range entries {
			if ent.IsDir() && !strings.HasPrefix(ent.Name(), ".") {
				_ = watcher.Add(filepath.Join(m.paths.SkillsDir(), ent.Name()))
			}
		}
	}

	go func() {
		defer watcher.Close()

		timer := time.NewTimer(watchDebounce)
		timer.Stop()
		var timerC <-chan time.Time
		var lastTouchedFile string

		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return

			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !m.shouldReloadForEvent(ev, watcher) {
					continue
				}
				lastTouchedFile = ev.Name
				timer.Reset(watchDebounce)
				timerC = timer.C

			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if onChange != nil && watchErr != nil {
					onChange(m.Current(), &ReloadDiagnostic{
						File:      m.paths.Root,
						Message:   watchErr.Error(),
						Err:       watchErr,
						Timestamp: m.now(),
					})
				}

			case <-timerC:
				timerC = nil
				newSnap, loadErr := m.loadGlobalSnapshot()
				if loadErr != nil {
					diagFile := lastTouchedFile
					var fe *fileLoadError
					if errors.As(loadErr, &fe) && fe.File != "" {
						diagFile = fe.File
					}
					if onChange != nil {
						onChange(m.Current(), &ReloadDiagnostic{
							File:      diagFile,
							Message:   loadErr.Error(),
							Err:       loadErr,
							Timestamp: m.now(),
						})
					}
					continue
				}
				m.current.Store(newSnap)
				m.notifyReload(newSnap)
				if onChange != nil {
					onChange(newSnap, nil)
				}
			}
		}
	}()

	return nil
}

func (m *Manager) shouldReloadForEvent(ev fsnotify.Event, watcher *fsnotify.Watcher) bool {
	// 忽略纯权限变更事件，防止内部 chmod 0600 触发循环重载。
	if ev.Op == fsnotify.Chmod {
		return false
	}
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}

	cleanPath := filepath.Clean(ev.Name)
	base := filepath.Base(cleanPath)
	if base == ".DS_Store" ||
		strings.HasSuffix(base, ".tmp") ||
		strings.HasSuffix(base, ".lock") ||
		strings.HasSuffix(base, ".swp") ||
		strings.HasSuffix(base, "~") ||
		strings.HasPrefix(base, ".#") {
		return false
	}

	// 排除运行时、会话存储区与常驻收发队列变动对配置引擎的干扰。
	for _, excluded := range []string{
		m.paths.RuntimeDir(),
		m.paths.WorkspacesDir(),
		m.paths.AlwaysOnDir(),
		m.paths.MemoryDir(),
	} {
		if cleanPath == excluded || strings.HasPrefix(cleanPath, excluded+string(filepath.Separator)) {
			return false
		}
	}

	// 若在 skills/ 下新增了技能子目录，动态挂载监听并触发防抖重载，防止同批次创建的 SKILL.md 漏事件。
	if ev.Op&fsnotify.Create != 0 && filepath.Dir(cleanPath) == m.paths.SkillsDir() {
		if st, err := os.Stat(cleanPath); err == nil && st.IsDir() && !strings.HasPrefix(base, ".") {
			_ = watcher.Add(cleanPath)
			return true
		}
	}

	ext := filepath.Ext(base)
	return ext == ".toml" || ext == ".md"
}
