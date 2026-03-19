// Package metrics provides Prometheus metrics for the Mantle platform.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WorkflowExecutionsTotal counts completed workflow executions by workflow name and status.
	WorkflowExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mantle_workflow_executions_total",
			Help: "Total number of workflow executions by workflow and status.",
		},
		[]string{"workflow", "status"},
	)

	// StepDurationSeconds tracks the duration of individual step executions.
	StepDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mantle_step_duration_seconds",
			Help:    "Duration of step executions in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"workflow", "step", "action"},
	)

	// StepExecutionsTotal counts completed step executions by workflow, step, and status.
	StepExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mantle_step_executions_total",
			Help: "Total number of step executions by workflow, step, and status.",
		},
		[]string{"workflow", "step", "status"},
	)

	// ConnectorDurationSeconds tracks the duration of connector invocations.
	ConnectorDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mantle_connector_duration_seconds",
			Help:    "Duration of connector invocations in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"action"},
	)

	// ActiveExecutions tracks the number of currently running executions.
	ActiveExecutions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mantle_active_executions",
			Help: "Number of currently running workflow executions.",
		},
	)
)

// RecordExecution increments the workflow execution counter for the given workflow and status.
func RecordExecution(workflow, status string) {
	WorkflowExecutionsTotal.WithLabelValues(workflow, status).Inc()
}

// RecordStepDuration observes the duration of a step execution.
func RecordStepDuration(workflow, step, action string, duration time.Duration) {
	StepDurationSeconds.WithLabelValues(workflow, step, action).Observe(duration.Seconds())
}

// RecordStepExecution increments the step execution counter for the given workflow, step, and status.
func RecordStepExecution(workflow, step, status string) {
	StepExecutionsTotal.WithLabelValues(workflow, step, status).Inc()
}

// RecordConnectorDuration observes the duration of a connector invocation.
func RecordConnectorDuration(action string, duration time.Duration) {
	ConnectorDurationSeconds.WithLabelValues(action).Observe(duration.Seconds())
}

// SetActiveExecutions sets the gauge for the number of active executions.
func SetActiveExecutions(n int) {
	ActiveExecutions.Set(float64(n))
}
