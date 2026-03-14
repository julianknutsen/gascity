package workspacesvc

import (
	"fmt"

	"github.com/gastownhall/gascity/internal/config"
)

// ValidateRuntimeSupport rejects service configs that the current controller
// binary cannot activate.
func ValidateRuntimeSupport(services []config.Service) error {
	for _, svc := range services {
		if svc.KindOrDefault() != "workflow" {
			return fmt.Errorf("service %q: unsupported kind %q", svc.Name, svc.KindOrDefault())
		}
		if lookupWorkflowContract(svc.Workflow.Contract) == nil {
			return fmt.Errorf("service %q: unsupported workflow contract %q", svc.Name, svc.Workflow.Contract)
		}
	}
	return nil
}
