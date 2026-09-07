package main

import (
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
)

// executorIdentityResidueCheck is a gc doctor check for stale
// executor-identity stamp residue: TODO(ga-cm2o5t.1.2) implement in the
// GREEN step.
type executorIdentityResidueCheck struct {
	cfg      *config.City
	cityPath string
	newStore func(string) (beads.Store, error)
}

func newExecutorIdentityResidueCheck(cfg *config.City, cityPath string, newStore func(string) (beads.Store, error)) *executorIdentityResidueCheck {
	return &executorIdentityResidueCheck{cfg: cfg, cityPath: cityPath, newStore: newStore}
}

func (c *executorIdentityResidueCheck) Name() string { return "executor-identity-residue" }

func (c *executorIdentityResidueCheck) CanFix() bool { return true }

func (c *executorIdentityResidueCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	return okCheck(c.Name(), "not yet implemented")
}

func (c *executorIdentityResidueCheck) Fix(_ *doctor.CheckContext) error {
	return nil
}
