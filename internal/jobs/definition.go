package jobs

import (
	"encoding/json"
	"fmt"
)

type Definition struct {
	Name           string            `json:"name"`
	Command        string            `json:"command"`
	Arguments      string            `json:"arguments"`
	Environment    map[string]string `json:"environment"`
	Priority       int               `json:"priority"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	MaxRetries     int               `json:"max_retries"`
	IdempotencyKey string            `json:"idempotency_key"`
	CreatedBy      string            `json:"created_by"`
	Script         string            `json:"script"`
	ScriptLanguage string            `json:"script_language"`
	TargetWorkerID string            `json:"target_worker_id"`
}

func ParseDefinition(s string) (*CreateJob, error) {
	var d Definition
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		return nil, fmt.Errorf("invalid job_definition: %w", err)
	}
	if d.Name == "" {
		return nil, fmt.Errorf("job definition requires name")
	}
	if d.Command == "" && d.Script == "" {
		return nil, fmt.Errorf("job definition requires either command or script")
	}
	var envStr string
	if len(d.Environment) > 0 {
		b, _ := json.Marshal(d.Environment)
		envStr = string(b)
	}
	return &CreateJob{
		Name:           d.Name,
		Command:        d.Command,
		Arguments:      d.Arguments,
		Environment:    envStr,
		Priority:       d.Priority,
		TimeoutSeconds: d.TimeoutSeconds,
		MaxRetries:     d.MaxRetries,
		IdempotencyKey: d.IdempotencyKey,
		CreatedBy:      d.CreatedBy,
		Script:         d.Script,
		ScriptLanguage: d.ScriptLanguage,
		TargetWorkerID: d.TargetWorkerID,
	}, nil
}
