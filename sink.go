package revenium

import (
	"context"

	"goa.design/goa-ai/runtime/agent/stream"
)

// Compile-time assertion that MeteringSink implements stream.Sink.
var _ stream.Sink = (*MeteringSink)(nil)

// MeteringSink wraps a stream.Sink to observe tool and workflow events for metering.
type MeteringSink struct {
	// Inner is the wrapped sink.
	Inner stream.Sink

	// Meter is the metering client.
	Meter *Meter
}

func (s *MeteringSink) Send(ctx context.Context, event stream.Event) error {
	// Skip metering if Meter is not configured
	if s.Meter == nil {
		return s.Inner.Send(ctx, event)
	}

	switch e := event.(type) {
	case stream.ToolStart:
		s.Meter.logger.Debug("tool start: %s (call_id=%s)", e.Data.ToolName, e.Data.ToolCallID)

	case stream.ToolEnd:
		s.Meter.logger.Debug("tool end: %s (call_id=%s, duration=%s)",
			e.Data.ToolName, e.Data.ToolCallID, e.Data.Duration)

	case stream.Workflow:
		s.Meter.logger.Debug("workflow phase: %s (status=%s)", e.Data.Phase, e.Data.Status)
		// Clean up trace registry on terminal workflow phases.
		switch e.Data.Phase {
		case "completed", "failed", "canceled":
			s.Meter.UnregisterTrace(e.RunID())
		}

	case stream.ChildRunLinked:
		s.Meter.logger.Debug("child run linked: agent=%s run=%s (parent_call=%s)",
			e.Data.ChildAgentID, e.Data.ChildRunID, e.Data.ToolCallID)
		// Pre-register the child run's trace mapping so the child planner
		// can inherit the parent's traceID before its PlanStart runs.
		if tc := GetTraceContext(ctx); tc != nil {
			s.Meter.RegisterTrace(e.Data.ChildRunID, tc.TraceID)
			s.Meter.logger.Debug("pre-registered child trace: child_run=%s trace=%s (parent_run=%s)",
				e.Data.ChildRunID, tc.TraceID, e.RunID())
		}

	case stream.Usage:
		s.Meter.logger.Debug("usage: model=%s input=%d output=%d total=%d",
			e.Data.Model, e.Data.InputTokens, e.Data.OutputTokens, e.Data.TotalTokens)

	default:
		// Unknown event types are passed through without logging to avoid noise
	}

	return s.Inner.Send(ctx, event)
}

func (s *MeteringSink) Close(ctx context.Context) error {
	return s.Inner.Close(ctx)
}
