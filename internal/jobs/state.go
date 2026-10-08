package jobs

import "errors"

var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict")
)

const (
	StatePending   = "PENDING"
	StateQueued    = "QUEUED"
	StateRunning   = "RUNNING"
	StateSucceeded = "SUCCEEDED"
	StateFailed    = "FAILED"
	StateRetrying  = "RETRYING"
	StateLost      = "LOST"
)

var valid = map[string]map[string]bool{
	StatePending:   {StateQueued: true},
	StateQueued:    {StateRunning: true},
	StateRunning:   {StateSucceeded: true, StateFailed: true, StateRetrying: true, StateLost: true},
	StateRetrying:  {StateQueued: true},
	StateLost:      {StateRetrying: true, StateFailed: true},
	StateFailed:    {StateQueued: true},
	StateSucceeded: {},
}

func CanTransition(from, to string) bool {
	if m, ok := valid[from]; ok {
		return m[to]
	}
	return false
}

type StateMachine struct{}

func NewStateMachine() *StateMachine { return &StateMachine{} }

func (s *StateMachine) Validate(from, to string) error {
	if !CanTransition(from, to) {
		return ErrInvalidTransition
	}
	return nil
}
