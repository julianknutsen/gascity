package main

import (
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

func TestProjectBdActorForMutationUsesOwnedIdentitySet(t *testing.T) {
	const (
		sessionID = "gcg-session-abc123"
		slot      = "city--mechanic-4-pool"
	)
	identities := []string{sessionID, slot}

	tests := []struct {
		name         string
		args         []string
		ambientActor string
		claimActor   string
		targets      map[string]beads.Bead
		want         string
	}{
		{
			name:         "fresh claim uses the canonical hook actor",
			args:         []string{"update", "work-1", "--claim"},
			ambientActor: slot,
			claimActor:   sessionID,
			targets:      map[string]beads.Bead{"work-1": {ID: "work-1"}},
			want:         sessionID,
		},
		{
			name:         "close projects the owned session identity",
			args:         []string{"close", "work-1"},
			ambientActor: slot,
			targets:      map[string]beads.Bead{"work-1": {ID: "work-1", Assignee: sessionID}},
			want:         sessionID,
		},
		{
			name:         "heartbeat projects the owned session identity",
			args:         []string{"heartbeat", "work-1"},
			ambientActor: slot,
			targets:      map[string]beads.Bead{"work-1": {ID: "work-1", Assignee: sessionID}},
			want:         sessionID,
		},
		{
			name:         "foreign owner stays foreign",
			args:         []string{"close", "work-1"},
			ambientActor: slot,
			targets:      map[string]beads.Bead{"work-1": {ID: "work-1", Assignee: "other-session"}},
			want:         slot,
		},
		{
			name:         "mixed owned identities do not impersonate a batch",
			args:         []string{"close", "work-1", "work-2"},
			ambientActor: slot,
			targets: map[string]beads.Bead{
				"work-1": {ID: "work-1", Assignee: sessionID},
				"work-2": {ID: "work-2", Assignee: slot},
			},
			want: slot,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectBdActorForMutation(tc.args, tc.ambientActor, tc.claimActor, identities, tc.targets); got != tc.want {
				t.Fatalf("projectBdActorForMutation() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBdSessionIdentityCandidatesPreserveNamedAndPoolForms(t *testing.T) {
	env := []string{
		"GC_SESSION_ID=gcg-session-abc123",
		"GC_SESSION_NAME=city--mechanic-4-pool",
		"GC_ALIAS=",
		"GC_AGENT=city--mechanic-4-pool",
		"BEADS_ACTOR=city--mechanic-4-pool",
	}
	got := bdSessionIdentityCandidates(env)
	for _, want := range []string{
		"gcg-session-abc123",
		"city--mechanic-4-pool",
	} {
		if !containsString(got, want) {
			t.Fatalf("bdSessionIdentityCandidates() = %q, missing %q", got, want)
		}
	}

	named := []string{
		"GC_SESSION_ID=gcg-session-mayor",
		"GC_SESSION_NAME=city--mayor",
		"GC_ALIAS=city/mayor",
		"GC_AGENT=city/mayor",
		"BEADS_ACTOR=city/mayor",
	}
	if got := bdSessionIdentityCandidates(named); !containsString(got, "city/mayor") {
		t.Fatalf("named bdSessionIdentityCandidates() = %q, missing configured alias", got)
	}
}
