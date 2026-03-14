package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/workspacesvc"
)

type fakeServiceRegistry struct {
	items []workspacesvc.Status
	serve func(w http.ResponseWriter, r *http.Request) bool
}

func (f *fakeServiceRegistry) List() []workspacesvc.Status {
	out := make([]workspacesvc.Status, len(f.items))
	copy(out, f.items)
	return out
}

func (f *fakeServiceRegistry) Get(name string) (workspacesvc.Status, bool) {
	for _, item := range f.items {
		if item.ServiceName == name {
			return item, true
		}
	}
	return workspacesvc.Status{}, false
}

func (f *fakeServiceRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) bool {
	if f.serve == nil {
		return false
	}
	return f.serve(w, r)
}

func TestHandleServicesListAndGet(t *testing.T) {
	state := newFakeState(t)
	state.services = &fakeServiceRegistry{
		items: []workspacesvc.Status{{
			ServiceName:      "review-intake",
			Kind:             "workflow",
			WorkflowContract: "pack.gc/review-intake.v1",
			MountPath:        "/svc/review-intake",
			PublishMode:      "private",
			StateRoot:        ".gc/services/review-intake",
			ServiceState:     "ready",
			LocalState:       "ready",
			PublicationState: "private",
		}},
	}
	srv := New(state)

	listReq := httptest.NewRequest(http.MethodGet, "/v0/services", nil)
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var listResp struct {
		Items []workspacesvc.Status `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listResp.Total != 1 {
		t.Fatalf("Total = %d, want 1", listResp.Total)
	}
	if len(listResp.Items) != 1 || listResp.Items[0].ServiceName != "review-intake" {
		t.Fatalf("Items = %#v, want review-intake", listResp.Items)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/service/review-intake", nil)
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}
	var got workspacesvc.Status
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.ServiceName != "review-intake" {
		t.Errorf("ServiceName = %q, want review-intake", got.ServiceName)
	}
}

func TestServiceProxyDirectAllowsExternalMutationWithoutCSRF(t *testing.T) {
	state := newFakeState(t)
	state.services = &fakeServiceRegistry{
		items: []workspacesvc.Status{{
			ServiceName: "review-intake",
			PublishMode: "direct",
		}},
		serve: func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/svc/review-intake/hooks/github" {
				t.Errorf("path = %q, want /svc/review-intake/hooks/github", r.URL.Path)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("proxied"))
			return true
		},
	}
	srv := NewReadOnly(state)

	req := httptest.NewRequest(http.MethodPost, "/svc/review-intake/hooks/github", strings.NewReader(`{}`))
	req.RemoteAddr = "198.51.100.10:9000"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if strings.TrimSpace(rec.Body.String()) != "proxied" {
		t.Errorf("body = %q, want proxied", rec.Body.String())
	}
}

func TestServiceProxyPrivateRejectsExternalRequests(t *testing.T) {
	state := newFakeState(t)
	state.services = &fakeServiceRegistry{
		items: []workspacesvc.Status{{
			ServiceName: "review-intake",
			PublishMode: "private",
		}},
		serve: func(http.ResponseWriter, *http.Request) bool {
			t.Fatal("service should not have been invoked for external private request")
			return false
		},
	}
	srv := New(state)

	req := httptest.NewRequest(http.MethodPost, "/svc/review-intake/hooks/github", strings.NewReader(`{}`))
	req.RemoteAddr = "198.51.100.10:9000"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServiceProxyPrivateRequiresCSRFForLocalMutation(t *testing.T) {
	state := newFakeState(t)
	state.services = &fakeServiceRegistry{
		items: []workspacesvc.Status{{
			ServiceName: "review-intake",
			PublishMode: "private",
		}},
		serve: func(http.ResponseWriter, *http.Request) bool {
			t.Fatal("service should not have been invoked without CSRF header")
			return false
		},
	}
	srv := New(state)

	req := httptest.NewRequest(http.MethodPost, "/svc/review-intake/hooks/github", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:9000"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestServiceProxyPrivateAllowsLocalMutationWithCSRF(t *testing.T) {
	state := newFakeState(t)
	state.services = &fakeServiceRegistry{
		items: []workspacesvc.Status{{
			ServiceName: "review-intake",
			PublishMode: "private",
		}},
		serve: func(w http.ResponseWriter, _ *http.Request) bool {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("proxied"))
			return true
		},
	}
	srv := New(state)

	req := httptest.NewRequest(http.MethodPost, "/svc/review-intake/hooks/github", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:9000"
	req.Header.Set("X-GC-Request", "1")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
}
