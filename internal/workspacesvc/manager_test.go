package workspacesvc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

type testRuntime struct {
	cityPath string
	cityName string
	cfg      *config.City
	sp       runtime.Provider
	store    beads.Store
}

func (t *testRuntime) CityPath() string                  { return t.cityPath }
func (t *testRuntime) CityName() string                  { return t.cityName }
func (t *testRuntime) Config() *config.City              { return t.cfg }
func (t *testRuntime) SessionProvider() runtime.Provider { return t.sp }
func (t *testRuntime) BeadStore(string) beads.Store      { return t.store }
func (t *testRuntime) Poke()                             {}

type testInstance struct {
	status     Status
	handleHTTP func(w http.ResponseWriter, r *http.Request, subpath string) bool
	closed     bool
}

func (t *testInstance) Status() Status { return t.status }

func (t *testInstance) HandleHTTP(w http.ResponseWriter, r *http.Request, subpath string) bool {
	if t.handleHTTP == nil {
		return false
	}
	return t.handleHTTP(w, r, subpath)
}

func (t *testInstance) Tick(context.Context, time.Time) {}

func (t *testInstance) Close() error {
	t.closed = true
	return nil
}

func uniqueContract(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test.%s.%d", strings.ReplaceAll(t.Name(), "/", "."), time.Now().UnixNano())
}

func TestManagerReloadWorkflowServiceCreatesStateRootAndDirectURL(t *testing.T) {
	contract := uniqueContract(t)
	RegisterWorkflowContract(contract, func(_ RuntimeContext, svc config.Service) (Instance, error) {
		return &testInstance{
			status: Status{
				ServiceName:      svc.Name,
				WorkflowContract: contract,
				ServiceState:     "ready",
				LocalState:       "ready",
			},
		}, nil
	})

	rt := &testRuntime{
		cityPath: t.TempDir(),
		cityName: "test-city",
		cfg: &config.City{
			API: config.APIConfig{Bind: "127.0.0.1", Port: 9443},
			Services: []config.Service{{
				Name:        "review-intake",
				PublishMode: "direct",
				Workflow:    config.ServiceWorkflowConfig{Contract: contract},
			}},
		},
		sp:    runtime.NewFake(),
		store: beads.NewMemStore(),
	}

	mgr := NewManager(rt)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	status, ok := mgr.Get("review-intake")
	if !ok {
		t.Fatal("service status missing")
	}
	if status.PublicURL != "http://127.0.0.1:9443/svc/review-intake" {
		t.Errorf("PublicURL = %q, want %q", status.PublicURL, "http://127.0.0.1:9443/svc/review-intake")
	}
	if status.PublicationState != "direct" {
		t.Errorf("PublicationState = %q, want direct", status.PublicationState)
	}

	stateRoot := filepath.Join(rt.cityPath, status.StateRoot)
	for _, want := range []string{
		stateRoot,
		filepath.Join(stateRoot, "data"),
		filepath.Join(stateRoot, "run"),
		filepath.Join(stateRoot, "logs"),
		filepath.Join(stateRoot, "secrets"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("expected %s to exist: %v", want, err)
		}
	}
}

func TestManagerReloadUnsupportedContractDegradesService(t *testing.T) {
	rt := &testRuntime{
		cityPath: t.TempDir(),
		cityName: "test-city",
		cfg: &config.City{
			Services: []config.Service{{
				Name:     "review-intake",
				Workflow: config.ServiceWorkflowConfig{Contract: "missing.contract"},
			}},
		},
		sp:    runtime.NewFake(),
		store: beads.NewMemStore(),
	}

	mgr := NewManager(rt)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	status, ok := mgr.Get("review-intake")
	if !ok {
		t.Fatal("service status missing")
	}
	if status.ServiceState != "degraded" {
		t.Errorf("ServiceState = %q, want degraded", status.ServiceState)
	}
	if status.LocalState != "config_error" {
		t.Errorf("LocalState = %q, want config_error", status.LocalState)
	}
	if !strings.Contains(status.StateReason, "unsupported workflow contract") {
		t.Errorf("StateReason = %q, want unsupported workflow contract", status.StateReason)
	}
}

func TestManagerReloadClosesReplacedInstances(t *testing.T) {
	contract := uniqueContract(t)
	first := &testInstance{}
	second := &testInstance{}
	callCount := 0
	RegisterWorkflowContract(contract, func(_ RuntimeContext, svc config.Service) (Instance, error) {
		callCount++
		inst := first
		if callCount > 1 {
			inst = second
		}
		inst.status = Status{
			ServiceName:      svc.Name,
			WorkflowContract: contract,
			ServiceState:     "ready",
			LocalState:       "ready",
		}
		return inst, nil
	})

	rt := &testRuntime{
		cityPath: t.TempDir(),
		cityName: "test-city",
		cfg: &config.City{
			Services: []config.Service{{
				Name:     "review-intake",
				Workflow: config.ServiceWorkflowConfig{Contract: contract},
			}},
		},
		sp:    runtime.NewFake(),
		store: beads.NewMemStore(),
	}

	mgr := NewManager(rt)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("first Reload: %v", err)
	}
	if err := mgr.Reload(); err != nil {
		t.Fatalf("second Reload: %v", err)
	}
	if !first.closed {
		t.Fatal("expected first instance to be closed on replacement reload")
	}
	if second.closed {
		t.Fatal("expected current instance to remain open")
	}
}

func TestManagerServeHTTPRoutesToWorkflowInstance(t *testing.T) {
	contract := uniqueContract(t)
	RegisterWorkflowContract(contract, func(_ RuntimeContext, svc config.Service) (Instance, error) {
		return &testInstance{
			status: Status{
				ServiceName:      svc.Name,
				WorkflowContract: contract,
				ServiceState:     "ready",
				LocalState:       "ready",
			},
			handleHTTP: func(w http.ResponseWriter, r *http.Request, subpath string) bool {
				if subpath != "/hooks/github" {
					t.Errorf("subpath = %q, want /hooks/github", subpath)
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(r.Method + " " + subpath))
				return true
			},
		}, nil
	})

	rt := &testRuntime{
		cityPath: t.TempDir(),
		cityName: "test-city",
		cfg: &config.City{
			Services: []config.Service{{
				Name:     "review-intake",
				Workflow: config.ServiceWorkflowConfig{Contract: contract},
			}},
		},
		sp:    runtime.NewFake(),
		store: beads.NewMemStore(),
	}

	mgr := NewManager(rt)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/svc/review-intake/hooks/github", nil)
	rec := httptest.NewRecorder()
	if ok := mgr.ServeHTTP(rec, req); !ok {
		t.Fatal("ServeHTTP returned false, want true")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if strings.TrimSpace(rec.Body.String()) != "POST /hooks/github" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "POST /hooks/github")
	}
}
