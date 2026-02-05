package revenium

import (
	"context"

	"github.com/google/uuid"
)

type contextKey struct{}

// TraceContext carries correlation and metering metadata through context.
type TraceContext struct {
	// TraceID is a correlation ID across the full request.
	TraceID string

	// TraceName is a human-readable trace name.
	TraceName string

	// TraceType classifies the trace (e.g., "agent", "tool").
	TraceType string

	// TransactionID is unique per LLM call or tool invocation.
	TransactionID string

	// ParentTxnID is the parent transaction for nesting.
	ParentTxnID string

	// Squad is the agent group identifier.
	Squad string
}

// WithTraceContext stores a TraceContext in the context.
// A shallow copy is made to avoid mutating the input struct.
func WithTraceContext(ctx context.Context, tc *TraceContext) context.Context {
	if tc == nil {
		tc = &TraceContext{}
	}
	// Create a shallow copy to avoid mutating the caller's struct
	tcCopy := *tc
	if tcCopy.TraceID == "" {
		tcCopy.TraceID = uuid.New().String()
	}
	return context.WithValue(ctx, contextKey{}, &tcCopy)
}

// GetTraceContext retrieves the TraceContext from the context, or nil if not set.
func GetTraceContext(ctx context.Context) *TraceContext {
	tc, _ := ctx.Value(contextKey{}).(*TraceContext)
	return tc
}
