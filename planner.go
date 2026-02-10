package revenium

import (
	"context"

	"github.com/google/uuid"

	"goa.design/goa-ai/runtime/agent/model"
	"goa.design/goa-ai/runtime/agent/planner"
	"goa.design/goa-ai/runtime/agent/run"
)

// MeteringPlanner wraps a planner.Planner to inject metering into LLM calls.
// It intercepts ModelClient() calls on the PlannerContext to wrap the returned
// model.Client with metering instrumentation.
type MeteringPlanner struct {
	// Inner is the wrapped planner.
	Inner planner.Planner

	// Meter is the metering client.
	Meter *Meter

	// AgentID identifies the agent for squad detection and trace metadata.
	AgentID string

	// Provider identifies the LLM provider (e.g., "OpenAI", "Anthropic").
	// If empty, defaults to "unknown".
	Provider string

	// CapturePrompts enables capturing system prompts, input messages, and
	// output responses in metering payloads. Disabled by default since
	// prompts may contain sensitive data.
	CapturePrompts bool
}

func (p *MeteringPlanner) PlanStart(ctx context.Context, input *planner.PlanInput) (*planner.PlanResult, error) {
	ctx = p.ensureTraceContext(ctx, input.RunContext)
	input.Agent = &meteringPlannerContext{
		PlannerContext: input.Agent,
		meter:          p.Meter,
		agentID:        p.AgentID,
		provider:       p.resolveProvider(),
		capturePrompts: p.CapturePrompts,
	}
	return p.Inner.PlanStart(ctx, input)
}

func (p *MeteringPlanner) PlanResume(ctx context.Context, input *planner.PlanResumeInput) (*planner.PlanResult, error) {
	ctx = p.ensureTraceContext(ctx, input.RunContext)
	input.Agent = &meteringPlannerContext{
		PlannerContext: input.Agent,
		meter:          p.Meter,
		agentID:        p.AgentID,
		provider:       p.resolveProvider(),
		capturePrompts: p.CapturePrompts,
	}
	return p.Inner.PlanResume(ctx, input)
}

func (p *MeteringPlanner) resolveProvider() string {
	if p.Provider != "" {
		return p.Provider
	}
	return "unknown"
}

func (p *MeteringPlanner) ensureTraceContext(ctx context.Context, rc run.Context) context.Context {
	tc := GetTraceContext(ctx)
	if tc != nil {
		return ctx
	}

	tc = &TraceContext{
		TraceType:     "agent",
		TransactionID: rc.RunID,
		Squad:         ResolveSquad(p.Meter.cfg, p.AgentID),
	}

	if rc.ParentRunID == "" {
		// Top-level run: generate a new traceID and register it.
		tc.TraceID = uuid.New().String()
		p.Meter.RegisterTrace(rc.RunID, tc.TraceID)
	} else {
		// Child run: inherit the parent's traceID and set ParentTxnID.
		if parentTraceID, ok := p.Meter.LookupTrace(rc.ParentRunID); ok {
			tc.TraceID = parentTraceID
		} else {
			// Fallback: parent trace not found, generate new traceID.
			p.Meter.logger.Warn("parent trace not found for run=%s parent=%s, generating new traceID", rc.RunID, rc.ParentRunID)
			tc.TraceID = uuid.New().String()
		}
		tc.ParentTxnID = rc.ParentRunID
		p.Meter.RegisterTrace(rc.RunID, tc.TraceID)
	}

	return WithTraceContext(ctx, tc)
}

// meteringPlannerContext wraps PlannerContext to intercept ModelClient() calls.
// All other PlannerContext methods are inherited from the embedded interface.
type meteringPlannerContext struct {
	planner.PlannerContext
	meter          *Meter
	agentID        string
	provider       string
	capturePrompts bool
}

func (m *meteringPlannerContext) ModelClient(id string) (model.Client, bool) {
	client, ok := m.PlannerContext.ModelClient(id)
	if !ok {
		return nil, false
	}
	return &meteringClient{
		inner:          client,
		meter:          m.meter,
		modelID:        id,
		agentID:        m.agentID,
		provider:       m.provider,
		capturePrompts: m.capturePrompts,
	}, true
}

// Compile-time interface satisfaction check.
var _ planner.PlannerContext = (*meteringPlannerContext)(nil)
