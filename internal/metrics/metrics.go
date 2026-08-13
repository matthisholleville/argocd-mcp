// Package metrics exposes Prometheus counters for MCP tool calls, so
// operators can track usage and latency on a dashboard without parsing
// audit logs. It is intentionally independent of AUDIT_LOG: metrics carry
// no user identity (only a bounded tool/status label set), so they stay on
// even when audit logging is disabled for volume or privacy reasons.
package metrics

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Collectors bundles the Prometheus collectors for MCP tool calls.
type Collectors struct {
	// ToolCalls counts tool invocations by tool name and outcome
	// (ok | blocked | error). No user label: with N users x M tools x 3
	// outcomes, adding user would blow up the series count for no real
	// operational benefit — the audit log already carries per-user detail.
	ToolCalls *prometheus.CounterVec

	// ToolDuration tracks tool call latency in seconds, by tool name only.
	ToolDuration *prometheus.HistogramVec
}

// New creates the collectors. They are not registered yet; call Register.
func New() *Collectors {
	return &Collectors{
		ToolCalls: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mcp_tool_calls_total",
				Help: "Total number of MCP tool calls.",
			},
			[]string{"tool", "status"},
		),
		ToolDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "mcp_tool_duration_seconds",
				Help: "Duration of completed MCP tool calls in seconds (excludes blocked/pre-flight-rejected calls, which never reach ArgoCD).",
				// prometheus.DefBuckets tops out at 10s, which collapses every
				// slow-but-normal ArgoCD operation (sync, rollout) into the +Inf
				// bucket and destroys p95/p99 resolution exactly where operators
				// need it most. Extend the tail to 120s.
				Buckets: append(append([]float64{}, prometheus.DefBuckets...), 30, 60, 120),
			},
			[]string{"tool"},
		),
	}
}

// Register registers every collector on reg. AlreadyRegisteredError is
// treated as success so the call is idempotent (safe to call more than
// once against the same registry, e.g. in tests).
func (c *Collectors) Register(reg prometheus.Registerer) error {
	if c == nil {
		return nil
	}
	for _, col := range []prometheus.Collector{c.ToolCalls, c.ToolDuration} {
		if err := reg.Register(col); err != nil {
			var alreadyErr prometheus.AlreadyRegisteredError
			if !errors.As(err, &alreadyErr) {
				return err
			}
		}
	}
	return nil
}

// StatusOf buckets a tool call outcome into the low-cardinality status
// label used by ToolCalls: "blocked" (rate limit / write-disabled /
// resource scope denial), "error" (the call failed), or "ok".
func StatusOf(blocked bool, err error) string {
	switch {
	case blocked:
		return "blocked"
	case err != nil:
		return "error"
	default:
		return "ok"
	}
}

// IncCall records one tool call's outcome, with no duration. Use this for
// calls that never reached ArgoCD (blocked by rate limit / DISABLE_WRITE /
// ALLOWED_RESOURCES, or rejected during request validation): they have no
// meaningful "duration of an ArgoCD round trip" to report, and recording a
// synthetic 0 would pollute ToolDuration with a spike at the low end,
// skewing p95/p99 away from real latency exactly when rejections are most
// frequent (e.g. under heavy rate limiting). Safe to call on a nil
// *Collectors (a no-op).
func (c *Collectors) IncCall(tool, status string) {
	if c == nil {
		return
	}
	c.ToolCalls.WithLabelValues(tool, status).Inc()
}

// Observe records one completed tool call's outcome and duration — use this
// only after an actual attempt against ArgoCD (successful or failed), never
// for a call blocked/rejected before reaching it (see IncCall). Safe to call
// on a nil *Collectors (a no-op).
func (c *Collectors) Observe(tool, status string, duration time.Duration) {
	if c == nil {
		return
	}
	c.ToolCalls.WithLabelValues(tool, status).Inc()
	c.ToolDuration.WithLabelValues(tool).Observe(duration.Seconds())
}
