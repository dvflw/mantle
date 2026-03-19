package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordExecution(t *testing.T) {
	RecordExecution("order-flow", "completed")
	RecordExecution("order-flow", "completed")
	RecordExecution("order-flow", "failed")

	if got := testutil.ToFloat64(WorkflowExecutionsTotal.WithLabelValues("order-flow", "completed")); got != 2 {
		t.Errorf("expected 2 completed executions, got %v", got)
	}
	if got := testutil.ToFloat64(WorkflowExecutionsTotal.WithLabelValues("order-flow", "failed")); got != 1 {
		t.Errorf("expected 1 failed execution, got %v", got)
	}
}

func TestRecordStepDuration(t *testing.T) {
	RecordStepDuration("order-flow", "fetch-data", "http.request", 150*time.Millisecond)
	RecordStepDuration("order-flow", "fetch-data", "http.request", 250*time.Millisecond)

	// Verify metrics were collected (histogram creates multiple time series per label set).
	count := testutil.CollectAndCount(StepDurationSeconds)
	if count == 0 {
		t.Error("expected non-zero metric count for step duration histogram")
	}
}

func TestRecordStepExecution(t *testing.T) {
	RecordStepExecution("order-flow", "fetch-data", "completed")
	RecordStepExecution("order-flow", "fetch-data", "failed")
	RecordStepExecution("order-flow", "fetch-data", "completed")

	if got := testutil.ToFloat64(StepExecutionsTotal.WithLabelValues("order-flow", "fetch-data", "completed")); got != 2 {
		t.Errorf("expected 2 completed step executions, got %v", got)
	}
	if got := testutil.ToFloat64(StepExecutionsTotal.WithLabelValues("order-flow", "fetch-data", "failed")); got != 1 {
		t.Errorf("expected 1 failed step execution, got %v", got)
	}
}

func TestRecordConnectorDuration(t *testing.T) {
	RecordConnectorDuration("http.request", 100*time.Millisecond)
	RecordConnectorDuration("http.request", 200*time.Millisecond)

	count := testutil.CollectAndCount(ConnectorDurationSeconds)
	if count == 0 {
		t.Error("expected non-zero metric count for connector duration histogram")
	}
}

func TestSetActiveExecutions(t *testing.T) {
	SetActiveExecutions(5)
	if got := testutil.ToFloat64(ActiveExecutions); got != 5 {
		t.Errorf("expected 5 active executions, got %v", got)
	}

	SetActiveExecutions(0)
	if got := testutil.ToFloat64(ActiveExecutions); got != 0 {
		t.Errorf("expected 0 active executions, got %v", got)
	}
}
