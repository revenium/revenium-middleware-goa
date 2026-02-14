package revenium

import "context"

// meteringContextKey is the context key for MeteringContext.
type meteringContextKey struct{}

// MeteringContext carries per-request metering metadata through context.
// This allows multi-tenant applications to set organization, subscription,
// product, and subscriber information on a per-request basis rather than
// using static configuration.
type MeteringContext struct {
	// OrganizationName identifies the organization for this request.
	OrganizationName string

	// SubscriptionID is the subscription identifier for this request.
	SubscriptionID string

	// ProductName is the product name for this request.
	ProductName string

	// Subscriber holds subscriber metadata for this request.
	Subscriber *SubscriberResource
}

// WithMeteringContext stores a MeteringContext in the context.
// A shallow copy is made to avoid mutating the input struct.
func WithMeteringContext(ctx context.Context, mc *MeteringContext) context.Context {
	if mc == nil {
		return ctx
	}
	// Create a shallow copy to avoid mutating the caller's struct
	mcCopy := *mc
	// Deep copy the Subscriber if present
	if mc.Subscriber != nil {
		subCopy := *mc.Subscriber
		if mc.Subscriber.Credential != nil {
			credCopy := *mc.Subscriber.Credential
			subCopy.Credential = &credCopy
		}
		mcCopy.Subscriber = &subCopy
	}
	return context.WithValue(ctx, meteringContextKey{}, &mcCopy)
}

// GetMeteringContext retrieves the MeteringContext from the context, or nil if not set.
func GetMeteringContext(ctx context.Context) *MeteringContext {
	mc, _ := ctx.Value(meteringContextKey{}).(*MeteringContext)
	return mc
}
