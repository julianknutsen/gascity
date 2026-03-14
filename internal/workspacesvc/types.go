// Package workspacesvc provides the generic workspace-owned service runtime.
package workspacesvc

import (
	"context"
	"net/http"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

// Status is the API-facing state projection for one workspace service.
type Status struct {
	ServiceName      string    `json:"service_name"`
	Kind             string    `json:"kind,omitempty"`
	WorkflowContract string    `json:"workflow_contract,omitempty"`
	MountPath        string    `json:"mount_path"`
	PublishMode      string    `json:"publish_mode"`
	StateRoot        string    `json:"state_root"`
	PublicURL        string    `json:"public_url,omitempty"`
	ServiceState     string    `json:"service_state"`
	LocalState       string    `json:"local_state"`
	PublicationState string    `json:"publication_state"`
	StateReason      string    `json:"state_reason,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RuntimeContext provides the runtime hooks a workspace service can use.
type RuntimeContext interface {
	CityPath() string
	CityName() string
	Config() *config.City
	SessionProvider() runtime.Provider
	BeadStore(rig string) beads.Store
	Poke()
}

// Instance is one runtime service implementation.
type Instance interface {
	Status() Status
	HandleHTTP(w http.ResponseWriter, r *http.Request, subpath string) bool
	Tick(ctx context.Context, now time.Time)
	Close() error
}

// Registry is the controller-owned workspace service registry.
type Registry interface {
	List() []Status
	Get(name string) (Status, bool)
	ServeHTTP(w http.ResponseWriter, r *http.Request) bool
}

// WorkflowFactory constructs a workflow service for a known contract.
type WorkflowFactory func(rt RuntimeContext, svc config.Service) (Instance, error)
