package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func counterValue(t *testing.T, vec *prometheus.CounterVec, labels prometheus.Labels) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := vec.With(labels).Write(m); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestStatusOf(t *testing.T) {
	cases := []struct {
		name    string
		blocked bool
		err     error
		want    string
	}{
		{"blocked wins over nil error", true, nil, "blocked"},
		{"error when not blocked", false, errTest, "error"},
		{"ok when neither", false, nil, "ok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StatusOf(c.blocked, c.err); got != c.want {
				t.Errorf("StatusOf(%v, %v) = %q, want %q", c.blocked, c.err, got, c.want)
			}
		})
	}
}

var errTest = &testError{"boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestObserve_IncrementsCountersAndDuration(t *testing.T) {
	c := New()

	c.Observe("sync_application", "ok", 250*time.Millisecond)
	c.Observe("sync_application", "ok", 100*time.Millisecond)
	c.Observe("sync_application", "error", 50*time.Millisecond)

	if got := counterValue(t, c.ToolCalls, prometheus.Labels{"tool": "sync_application", "status": "ok"}); got != 2 {
		t.Errorf("ok count = %v, want 2", got)
	}
	if got := counterValue(t, c.ToolCalls, prometheus.Labels{"tool": "sync_application", "status": "error"}); got != 1 {
		t.Errorf("error count = %v, want 1", got)
	}

	m := &dto.Metric{}
	if err := c.ToolDuration.With(prometheus.Labels{"tool": "sync_application"}).(prometheus.Histogram).Write(m); err != nil {
		t.Fatalf("write histogram: %v", err)
	}
	if got := m.GetHistogram().GetSampleCount(); got != 3 {
		t.Errorf("duration sample count = %v, want 3", got)
	}
}

func TestIncCall_CountsWithoutRecordingDuration(t *testing.T) {
	c := New()

	c.IncCall("execute_operation", "blocked")
	c.IncCall("execute_operation", "blocked")

	if got := counterValue(t, c.ToolCalls, prometheus.Labels{"tool": "execute_operation", "status": "blocked"}); got != 2 {
		t.Errorf("blocked count = %v, want 2", got)
	}

	m := &dto.Metric{}
	if err := c.ToolDuration.With(prometheus.Labels{"tool": "execute_operation"}).(prometheus.Histogram).Write(m); err != nil {
		t.Fatalf("write histogram: %v", err)
	}
	if got := m.GetHistogram().GetSampleCount(); got != 0 {
		t.Errorf("duration sample count = %v, want 0 (blocked calls must not pollute latency histogram)", got)
	}
}

func TestIncCall_NilCollectorsIsNoop(t *testing.T) {
	var c *Collectors
	// Must not panic.
	c.IncCall("any_tool", "blocked")
}

func TestObserve_NilCollectorsIsNoop(t *testing.T) {
	var c *Collectors
	// Must not panic.
	c.Observe("any_tool", "ok", time.Second)
}

func TestRegister_IdempotentOnDoubleRegistration(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := New()

	if err := c.Register(reg); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := c.Register(reg); err != nil {
		t.Fatalf("second Register (should be idempotent): %v", err)
	}
}

func TestRegister_NilCollectorsIsNoop(t *testing.T) {
	var c *Collectors
	reg := prometheus.NewRegistry()
	if err := c.Register(reg); err != nil {
		t.Errorf("Register on nil Collectors: %v", err)
	}
}
