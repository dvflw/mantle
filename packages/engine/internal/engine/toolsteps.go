package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ToolSteps provides sub-step DB persistence and LLM response caching
// for crash recovery during multi-step tool executions.
type ToolSteps struct {
	DB *sql.DB
}

// CreateSubStep inserts a child step_execution with a parent_step_id reference.
// It uses ON CONFLICT DO NOTHING for idempotent creation — calling this twice
// with the same (executionID, stepName, attempt) is a no-op on the second call.
func (ts *ToolSteps) CreateSubStep(ctx context.Context, executionID, parentStepID, stepName string, maxAttempts int) (string, error) {
	var id string
	err := ts.DB.QueryRowContext(ctx, `
		INSERT INTO step_executions (execution_id, step_name, attempt, status, parent_step_id)
		VALUES ($1, $2, 1, 'pending', $3)
		ON CONFLICT (execution_id, step_name, attempt) WHERE hook_block IS NULL DO NOTHING
		RETURNING id
	`, executionID, stepName, parentStepID).Scan(&id)

	if err == sql.ErrNoRows {
		// Row already existed — fetch the existing ID.
		err = ts.DB.QueryRowContext(ctx, `
			SELECT id FROM step_executions
			WHERE execution_id = $1 AND step_name = $2 AND attempt = 1
		`, executionID, stepName).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("fetching existing sub-step: %w", err)
		}
		return id, nil
	}
	if err != nil {
		return "", fmt.Errorf("creating sub-step: %w", err)
	}
	return id, nil
}

// CacheLLMResponse appends a response object to the cached_llm_responses
// JSONB array for the given step execution. Responses are appended in order
// so that replay during crash recovery produces the same sequence.
func (ts *ToolSteps) CacheLLMResponse(ctx context.Context, stepID string, response map[string]any) error {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshaling LLM response: %w", err)
	}

	result, err := ts.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET cached_llm_responses = cached_llm_responses || $2::jsonb
		WHERE id = $1
	`, stepID, data)
	if err != nil {
		return fmt.Errorf("caching LLM response: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("step execution %s not found", stepID)
	}
	return nil
}

// LoadCachedLLMResponses loads and unmarshals all cached LLM responses
// for a given step execution, preserving insertion order.
func (ts *ToolSteps) LoadCachedLLMResponses(ctx context.Context, stepID string) ([]map[string]any, error) {
	var raw []byte
	err := ts.DB.QueryRowContext(ctx, `
		SELECT cached_llm_responses FROM step_executions WHERE id = $1
	`, stepID).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("loading cached LLM responses: %w", err)
	}

	var responses []map[string]any
	if err := json.Unmarshal(raw, &responses); err != nil {
		return nil, fmt.Errorf("unmarshaling cached LLM responses: %w", err)
	}
	return responses, nil
}

// LoadSubStepStatuses loads all child step executions for a given parent
// and returns them as a map keyed by step_name.
func (ts *ToolSteps) LoadSubStepStatuses(ctx context.Context, parentStepID string) (map[string]*StepStatus, error) {
	rows, err := ts.DB.QueryContext(ctx, `
		SELECT step_name, status, attempt, output, error
		FROM step_executions
		WHERE parent_step_id = $1
		ORDER BY step_name, attempt
	`, parentStepID)
	if err != nil {
		return nil, fmt.Errorf("querying sub-step statuses: %w", err)
	}
	defer rows.Close()

	statuses := make(map[string]*StepStatus)
	for rows.Next() {
		var (
			stepName  string
			status    string
			attempt   int
			outputRaw []byte
			errStr    sql.NullString
		)
		if err := rows.Scan(&stepName, &status, &attempt, &outputRaw, &errStr); err != nil {
			return nil, fmt.Errorf("scanning sub-step row: %w", err)
		}

		ss := &StepStatus{
			Status:  status,
			Attempt: attempt,
		}
		if errStr.Valid {
			ss.Error = errStr.String
		}
		if outputRaw != nil {
			var output map[string]any
			if err := json.Unmarshal(outputRaw, &output); err != nil {
				return nil, fmt.Errorf("unmarshaling output for step %s: %w", stepName, err)
			}
			ss.Output = output
		}

		// Keep the latest attempt for each step name.
		if existing, ok := statuses[stepName]; !ok || attempt > existing.Attempt {
			statuses[stepName] = ss
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sub-step rows: %w", err)
	}
	return statuses, nil
}
