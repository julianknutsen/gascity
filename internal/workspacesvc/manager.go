package workspacesvc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gastownhall/gascity/internal/config"
)

// Manager owns the lifecycle and status projection for workspace services.
type Manager struct {
	rt RuntimeContext

	mu      sync.RWMutex
	entries map[string]*entry
}

type entry struct {
	spec   config.Service
	status Status
	inst   Instance
}

// NewManager creates a service manager bound to one workspace runtime.
func NewManager(rt RuntimeContext) *Manager {
	return &Manager{
		rt:      rt,
		entries: make(map[string]*entry),
	}
}

// Reload reconciles the manager against the current config snapshot.
func (m *Manager) Reload() error {
	cfg := m.rt.Config()
	next := make(map[string]*entry, len(cfg.Services))
	now := time.Now().UTC()

	for _, svc := range cfg.Services {
		base := baseStatus(m.rt.Config(), svc, now)
		stateRoot, err := ensureStateRoot(m.rt.CityPath(), svc)
		base.StateRoot = stateRoot
		if err != nil {
			base.ServiceState = "degraded"
			base.LocalState = "config_error"
			base.StateReason = err.Error()
			next[svc.Name] = &entry{spec: svc, status: base}
			continue
		}

		switch svc.KindOrDefault() {
		case "workflow":
			factory := lookupWorkflowContract(svc.Workflow.Contract)
			if factory == nil {
				base.ServiceState = "degraded"
				base.LocalState = "config_error"
				base.StateReason = fmt.Sprintf("unsupported workflow contract %q", svc.Workflow.Contract)
				next[svc.Name] = &entry{spec: svc, status: base}
				continue
			}
			inst, err := factory(m.rt, svc)
			if err != nil {
				base.ServiceState = "degraded"
				base.LocalState = "config_error"
				base.StateReason = err.Error()
				next[svc.Name] = &entry{spec: svc, status: base}
				continue
			}
			base = mergeStatus(base, inst.Status())
			next[svc.Name] = &entry{spec: svc, status: base, inst: inst}
		default:
			base.ServiceState = "degraded"
			base.LocalState = "config_error"
			base.StateReason = fmt.Sprintf("unsupported service kind %q", svc.Kind)
			next[svc.Name] = &entry{spec: svc, status: base}
		}
	}

	m.mu.Lock()
	old := m.entries
	m.entries = next
	m.mu.Unlock()

	var firstErr error
	for _, e := range old {
		if e.inst != nil {
			if err := e.inst.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// Tick runs one service reconciliation pass.
func (m *Manager) Tick(ctx context.Context, now time.Time) {
	m.mu.RLock()
	entries := make([]*entry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	m.mu.RUnlock()

	for _, e := range entries {
		if e.inst == nil {
			continue
		}
		e.inst.Tick(ctx, now)
		status := mergeStatus(baseStatus(m.rt.Config(), e.spec, now), e.inst.Status())
		m.mu.Lock()
		if cur, ok := m.entries[e.spec.Name]; ok {
			cur.status = status
		}
		m.mu.Unlock()
	}
}

// Close closes all runtime service instances.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for _, e := range m.entries {
		if e.inst == nil {
			continue
		}
		if err := e.inst.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// List returns all current service statuses sorted by name.
func (m *Manager) List() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Status, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e.status)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ServiceName < out[j].ServiceName
	})
	return out
}

// Get returns one current service status by name.
func (m *Manager) Get(name string) (Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[name]
	if !ok {
		return Status{}, false
	}
	return e.status, true
}

// ServeHTTP routes /svc/{name}/... requests to the matching service instance.
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) bool {
	path := strings.TrimPrefix(r.URL.Path, "/svc/")
	if path == r.URL.Path || path == "" {
		return false
	}

	name := path
	subpath := "/"
	if i := strings.IndexByte(path, '/'); i >= 0 {
		name = path[:i]
		subpath = path[i:]
	}
	if name == "" {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.entries[name]
	if !ok || e.inst == nil {
		return false
	}
	return e.inst.HandleHTTP(w, r, subpath)
}

func baseStatus(cfg *config.City, svc config.Service, now time.Time) Status {
	status := Status{
		ServiceName:      svc.Name,
		Kind:             svc.KindOrDefault(),
		WorkflowContract: svc.Workflow.Contract,
		MountPath:        svc.MountPathOrDefault(),
		PublishMode:      svc.PublishModeOrDefault(),
		StateRoot:        svc.StateRootOrDefault(),
		ServiceState:     "ready",
		LocalState:       "ready",
		PublicationState: "private",
		UpdatedAt:        now,
	}

	switch status.PublishMode {
	case "private":
		status.PublicationState = "private"
	case "direct":
		if baseURL := directBaseURL(cfg); baseURL != "" {
			status.PublicURL = strings.TrimRight(baseURL, "/") + status.MountPath
			status.PublicationState = "direct"
		} else {
			status.PublicationState = "blocked"
			status.StateReason = "direct_base_url_unavailable"
		}
	}

	return status
}

func mergeStatus(base, override Status) Status {
	if override.ServiceName != "" {
		base.ServiceName = override.ServiceName
	}
	if override.Kind != "" {
		base.Kind = override.Kind
	}
	if override.WorkflowContract != "" {
		base.WorkflowContract = override.WorkflowContract
	}
	if override.MountPath != "" {
		base.MountPath = override.MountPath
	}
	if override.PublishMode != "" {
		base.PublishMode = override.PublishMode
	}
	if override.StateRoot != "" {
		base.StateRoot = override.StateRoot
	}
	if override.PublicURL != "" {
		base.PublicURL = override.PublicURL
	}
	if override.ServiceState != "" {
		base.ServiceState = override.ServiceState
	}
	if override.LocalState != "" {
		base.LocalState = override.LocalState
	}
	if override.PublicationState != "" {
		base.PublicationState = override.PublicationState
	}
	if override.StateReason != "" {
		base.StateReason = override.StateReason
	}
	if !override.UpdatedAt.IsZero() {
		base.UpdatedAt = override.UpdatedAt
	}
	return base
}

func ensureStateRoot(cityPath string, svc config.Service) (string, error) {
	root := svc.StateRootOrDefault()
	absRoot := root
	if !filepath.IsAbs(absRoot) {
		absRoot = filepath.Join(cityPath, absRoot)
	}
	for _, dir := range []struct {
		path string
		mode os.FileMode
	}{
		{absRoot, 0o750},
		{filepath.Join(absRoot, "data"), 0o750},
		{filepath.Join(absRoot, "run"), 0o750},
		{filepath.Join(absRoot, "logs"), 0o750},
		{filepath.Join(absRoot, "secrets"), 0o700},
	} {
		if err := os.MkdirAll(dir.path, dir.mode); err != nil {
			return root, err
		}
		if err := os.Chmod(dir.path, dir.mode); err != nil {
			return root, err
		}
	}
	return root, nil
}

func directBaseURL(cfg *config.City) string {
	if cfg == nil || cfg.API.Port <= 0 {
		return ""
	}
	bind := cfg.API.BindOrDefault()
	switch bind {
	case "", "0.0.0.0", "::", "[::]":
		return ""
	}
	return "http://" + net.JoinHostPort(bind, strconv.Itoa(cfg.API.Port))
}
