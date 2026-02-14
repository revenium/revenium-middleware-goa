package revenium

import "context"

// MeteringContextOption is a functional option for configuring a MeteringContext.
type MeteringContextOption func(*MeteringContext)

// WithOrganization sets the organization name on the MeteringContext.
func WithOrganization(name string) MeteringContextOption {
	return func(mc *MeteringContext) {
		mc.OrganizationName = name
	}
}

// WithSubscription sets the subscription ID on the MeteringContext.
func WithSubscription(id string) MeteringContextOption {
	return func(mc *MeteringContext) {
		mc.SubscriptionID = id
	}
}

// WithProduct sets the product name on the MeteringContext.
func WithProduct(name string) MeteringContextOption {
	return func(mc *MeteringContext) {
		mc.ProductName = name
	}
}

// WithSubscriberInfo sets the subscriber ID and email on the MeteringContext.
func WithSubscriberInfo(id, email string) MeteringContextOption {
	return func(mc *MeteringContext) {
		if mc.Subscriber == nil {
			mc.Subscriber = &SubscriberResource{}
		}
		mc.Subscriber.ID = id
		mc.Subscriber.Email = email
	}
}

// WithSubscriberCredentialInfo sets the subscriber credential on the MeteringContext.
func WithSubscriberCredentialInfo(name, value string) MeteringContextOption {
	return func(mc *MeteringContext) {
		if mc.Subscriber == nil {
			mc.Subscriber = &SubscriberResource{}
		}
		mc.Subscriber.Credential = &CredentialResource{Name: name, Value: value}
	}
}

// NewMeteringContext creates a new MeteringContext with the given options.
func NewMeteringContext(opts ...MeteringContextOption) *MeteringContext {
	mc := &MeteringContext{}
	for _, opt := range opts {
		opt(mc)
	}
	return mc
}

// ContextWithMetering creates a new context with MeteringContext set using the given options.
// This is a convenience function that combines NewMeteringContext and WithMeteringContext.
func ContextWithMetering(ctx context.Context, opts ...MeteringContextOption) context.Context {
	return WithMeteringContext(ctx, NewMeteringContext(opts...))
}
