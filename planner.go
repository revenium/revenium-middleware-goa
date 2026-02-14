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

	// ModelName is the actual model name to report in metering payloads
	// (e.g., "gpt-4o", "claude-3-opus"). If empty, falls back to the
	// registered model ID.
	ModelName string

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
		modelName:      p.ModelName,
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
		modelName:      p.ModelName,
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
	existing := GetTraceContext(ctx)

	tc := &TraceContext{
		TraceType:     "agent",
		TransactionID: rc.RunID,
		Squad:         ResolveSquad(p.Meter.cfg, p.AgentID),
	}

	// If a TraceContext already exists, inherit its TraceID (allows shared tracing)
	if existing != nil && existing.TraceID != "" {
		tc.TraceID = existing.TraceID
		tc.TraceName = existing.TraceName
		// For child runs, also set parent transaction
		if rc.ParentRunID != "" {
			tc.ParentTxnID = rc.ParentRunID
		}
		p.Meter.RegisterTrace(rc.RunID, tc.TraceID)
		return WithTraceContext(ctx, tc)
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
	modelName      string
	capturePrompts bool
}

func (m *meteringPlannerContext) ModelClient(id string) (model.Client, bool) {
	client, ok := m.PlannerContext.ModelClient(id)
	if !ok {
		return nil, false
	}
	// Use configured modelName if set, otherwise fall back to registered model ID
	modelID := m.modelName
	if modelID == "" {
		modelID = id
	}
	return &meteringClient{
		inner:          client,
		meter:          m.meter,
		modelID:        modelID,
		agentID:        m.agentID,
		provider:       m.provider,
		capturePrompts: m.capturePrompts,
	}, true
}

// Compile-time interface satisfaction check.
var _ planner.PlannerContext = (*meteringPlannerContext)(nil)
