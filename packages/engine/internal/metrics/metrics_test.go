package metrics

import (
	"testing"
	"time"
)

func TestQueueMetrics(t *testing.T) {
	SetQueueDepth(5)
	RecordClaimDuration(100 * time.Millisecond)
	RecordLeaseRenewal()
	RecordLeaseExpiration()
	RecordReaperReclaimed(3)
}

func TestToolUseMetrics(t *testing.T) {
	RecordToolCall("agent", "get_weather", "completed")
	RecordToolRound("agent")
	RecordToolRoundDuration("agent", 500*time.Millisecond)
	RecordLLMCacheHit()
	SetParallelStepsInFlight(3)
}
