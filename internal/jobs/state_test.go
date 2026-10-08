package jobs

import (
	"testing"
)

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatePending, StateQueued, true},
		{StateQueued, StateRunning, true},
		{StateRunning, StateSucceeded, true},
		{StateRunning, StateFailed, true},
		{StateRunning, StateRetrying, true},
		{StateRetrying, StateQueued, true},
		{StateLost, StateRetrying, true},
		{StateLost, StateFailed, true},
		{StateFailed, StateQueued, true},
		{StateSucceeded, StateRunning, false},
		{StateFailed, StateSucceeded, false},
		{StatePending, StateSucceeded, false},
		{StateQueued, StateFailed, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Errorf("CanTransition(%q,%q)=%v want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestValidateRejectsInvalid(t *testing.T) {
	sm := NewStateMachine()
	if err := sm.Validate(StateQueued, StateSucceeded); err == nil {
		t.Error("expected error for QUEUED->SUCCEEDED")
	}
	if err := sm.Validate(StateQueued, StateRunning); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
