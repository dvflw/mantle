package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP request metrics.
var (
	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mantle_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path", "status"})
)

// Execution lifecycle metrics.
var (
	ExecutionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_executions_total",
		Help: "Total workflow executions by status",
	}, []string{"workflow", "status"})

	ExecutionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mantle_execution_duration_seconds",
		Help:    "Workflow execution duration in seconds",
		Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
	}, []string{"workflow"})

	StepsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_steps_total",
		Help: "Total step executions by status",
	}, []string{"workflow", "step", "status"})
)

// Queue and distribution metrics.
var (
	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mantle_queue_depth",
		Help: "Number of pending steps in the work queue",
	})
	ClaimDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "mantle_claim_duration_seconds",
		Help:    "Time from step pending to claimed",
		Buckets: prometheus.DefBuckets,
	})
	LeaseRenewalsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_lease_renewals_total",
		Help: "Total number of lease renewals",
	})
	LeaseExpirationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_lease_expirations_total",
		Help: "Total number of lease expirations (indicates node failures)",
	})
	ReaperReclaimedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_reaper_reclaimed_total",
		Help: "Total number of steps reclaimed by reaper",
	})
)

// AI/LLM observability metrics.
var (
	AITokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_ai_tokens_total",
		Help: "Total AI tokens consumed",
	}, []string{"workflow", "step", "model", "provider", "token_type"})
	// token_type: "prompt" or "completion"

	AIRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_ai_requests_total",
		Help: "Total AI provider API requests",
	}, []string{"workflow", "step", "model", "provider", "status"})
	// status: "success" or "error"

	AIRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mantle_ai_request_duration_seconds",
		Help:    "AI provider API request duration",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"workflow", "step", "model", "provider"})
)

// Tool-use metrics.
var (
	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_tool_calls_total",
		Help: "Total tool calls by step, tool, and status",
	}, []string{"step", "tool", "status"})

	ToolRoundsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_tool_rounds_total",
		Help: "Total tool use rounds by step",
	}, []string{"step"})

	ToolRoundDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mantle_tool_round_duration_seconds",
		Help:    "Duration of tool use rounds",
		Buckets: prometheus.DefBuckets,
	}, []string{"step"})

	LLMCacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_llm_cache_hits_total",
		Help: "Total LLM response cache hits during recovery",
	})

	ParallelStepsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mantle_parallel_steps_in_flight",
		Help: "Number of concurrent step executions per workflow",
	})
)

// Queue helper functions.

func SetQueueDepth(n int)                { QueueDepth.Set(float64(n)) }
func RecordClaimDuration(d time.Duration) { ClaimDurationSeconds.Observe(d.Seconds()) }
func RecordLeaseRenewal()                { LeaseRenewalsTotal.Inc() }
func RecordLeaseExpiration()             { LeaseExpirationsTotal.Inc() }
func RecordReaperReclaimed(n int)        { ReaperReclaimedTotal.Add(float64(n)) }

// Tool-use helper functions.

func RecordToolCall(step, tool, status string) {
	ToolCallsTotal.WithLabelValues(step, tool, status).Inc()
}
func RecordToolRound(step string) { ToolRoundsTotal.WithLabelValues(step).Inc() }
func RecordToolRoundDuration(step string, d time.Duration) {
	ToolRoundDurationSeconds.WithLabelValues(step).Observe(d.Seconds())
}
func RecordLLMCacheHit()          { LLMCacheHitsTotal.Inc() }
func SetParallelStepsInFlight(n int) { ParallelStepsInFlight.Set(float64(n)) }
