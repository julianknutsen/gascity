// Package beadstest provides a conformance test suite for beads.Store
// implementations. Each implementation's test file calls RunStoreTests
// with its own factory function.
package beadstest

import (
	"errors"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
)

// RunStoreTests runs the full conformance suite against a Store implementation.
// The newStore function must return a fresh, empty store for each call.
func RunStoreTests(t *testing.T, newStore func() beads.Store) {
	t.Helper()

	t.Run("CreateAssignsSequentialID", func(t *testing.T) {
		s := newStore()
		b1, err := s.Create(beads.Bead{Title: "first"})
		if err != nil {
			t.Fatal(err)
		}
		b2, err := s.Create(beads.Bead{Title: "second"})
		if err != nil {
			t.Fatal(err)
		}
		if b1.ID != "gc-1" {
			t.Errorf("first bead ID = %q, want %q", b1.ID, "gc-1")
		}
		if b2.ID != "gc-2" {
			t.Errorf("second bead ID = %q, want %q", b2.ID, "gc-2")
		}
	})

	t.Run("CreateSetsStatusOpen", func(t *testing.T) {
		s := newStore()
		b, err := s.Create(beads.Bead{Title: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if b.Status != "open" {
			t.Errorf("Status = %q, want %q", b.Status, "open")
		}
	})

	t.Run("CreateDefaultsTypeToTask", func(t *testing.T) {
		s := newStore()
		b, err := s.Create(beads.Bead{Title: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if b.Type != "task" {
			t.Errorf("Type = %q, want %q", b.Type, "task")
		}
	})

	t.Run("CreatePreservesExplicitType", func(t *testing.T) {
		s := newStore()
		b, err := s.Create(beads.Bead{Title: "test", Type: "bug"})
		if err != nil {
			t.Fatal(err)
		}
		if b.Type != "bug" {
			t.Errorf("Type = %q, want %q", b.Type, "bug")
		}
	})

	t.Run("CreateSetsCreatedAt", func(t *testing.T) {
		s := newStore()
		before := time.Now()
		b, err := s.Create(beads.Bead{Title: "test"})
		if err != nil {
			t.Fatal(err)
		}
		after := time.Now()
		if b.CreatedAt.Before(before) || b.CreatedAt.After(after) {
			t.Errorf("CreatedAt = %v, want between %v and %v", b.CreatedAt, before, after)
		}
	})

	t.Run("CreatePreservesTitle", func(t *testing.T) {
		s := newStore()
		b, err := s.Create(beads.Bead{Title: "Build a Tower of Hanoi app"})
		if err != nil {
			t.Fatal(err)
		}
		if b.Title != "Build a Tower of Hanoi app" {
			t.Errorf("Title = %q, want %q", b.Title, "Build a Tower of Hanoi app")
		}
	})

	t.Run("CreateAssigneeIsEmpty", func(t *testing.T) {
		s := newStore()
		b, err := s.Create(beads.Bead{Title: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if b.Assignee != "" {
			t.Errorf("Assignee = %q, want empty", b.Assignee)
		}
	})

	t.Run("GetExistingBead", func(t *testing.T) {
		s := newStore()
		created, err := s.Create(beads.Bead{Title: "round trip", Type: "bug"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != created.ID {
			t.Errorf("ID = %q, want %q", got.ID, created.ID)
		}
		if got.Title != created.Title {
			t.Errorf("Title = %q, want %q", got.Title, created.Title)
		}
		if got.Status != created.Status {
			t.Errorf("Status = %q, want %q", got.Status, created.Status)
		}
		if got.Type != created.Type {
			t.Errorf("Type = %q, want %q", got.Type, created.Type)
		}
		if !got.CreatedAt.Equal(created.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created.CreatedAt)
		}
		if got.Assignee != created.Assignee {
			t.Errorf("Assignee = %q, want %q", got.Assignee, created.Assignee)
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		s := newStore()
		// Create one bead so the store isn't empty, then look up a wrong ID.
		if _, err := s.Create(beads.Bead{Title: "exists"}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Get("gc-999")
		if err == nil {
			t.Fatal("Get(gc-999) should return error")
		}
		if !errors.Is(err, beads.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("GetNotFoundEmptyStore", func(t *testing.T) {
		s := newStore()
		_, err := s.Get("gc-1")
		if err == nil {
			t.Fatal("Get on empty store should return error")
		}
		if !errors.Is(err, beads.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("ListReturnsAllBeads", func(t *testing.T) {
		s := newStore()
		_, err := s.Create(beads.Bead{Title: "first"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Create(beads.Bead{Title: "second"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("List() returned %d beads, want 2", len(got))
		}
		if got[0].Title != "first" {
			t.Errorf("got[0].Title = %q, want %q", got[0].Title, "first")
		}
		if got[1].Title != "second" {
			t.Errorf("got[1].Title = %q, want %q", got[1].Title, "second")
		}
	})

	t.Run("ListEmptyStore", func(t *testing.T) {
		s := newStore()
		got, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("List() on empty store returned %d beads, want 0", len(got))
		}
	})

	t.Run("ListOrder", func(t *testing.T) {
		s := newStore()
		for _, title := range []string{"alpha", "beta", "gamma"} {
			if _, err := s.Create(beads.Bead{Title: title}); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatalf("List() returned %d beads, want 3", len(got))
		}
		want := []string{"alpha", "beta", "gamma"}
		for i, w := range want {
			if got[i].Title != w {
				t.Errorf("got[%d].Title = %q, want %q", i, got[i].Title, w)
			}
		}
	})

	t.Run("ReadyReturnsOpenBeads", func(t *testing.T) {
		s := newStore()
		_, err := s.Create(beads.Bead{Title: "first"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Create(beads.Bead{Title: "second"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.Ready()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("Ready() returned %d beads, want 2", len(got))
		}
		if got[0].Title != "first" {
			t.Errorf("got[0].Title = %q, want %q", got[0].Title, "first")
		}
		if got[1].Title != "second" {
			t.Errorf("got[1].Title = %q, want %q", got[1].Title, "second")
		}
	})

	t.Run("ReadyEmptyStore", func(t *testing.T) {
		s := newStore()
		got, err := s.Ready()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("Ready() on empty store returned %d beads, want 0", len(got))
		}
	})

	t.Run("ReadyOrder", func(t *testing.T) {
		s := newStore()
		for _, title := range []string{"alpha", "beta", "gamma"} {
			if _, err := s.Create(beads.Bead{Title: title}); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.Ready()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatalf("Ready() returned %d beads, want 3", len(got))
		}
		want := []string{"alpha", "beta", "gamma"}
		for i, w := range want {
			if got[i].Title != w {
				t.Errorf("got[%d].Title = %q, want %q", i, got[i].Title, w)
			}
		}
	})
}
