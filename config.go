package revenium

import (
	"net/http"
	"os"
	"strings"
)

const (
	defaultBaseURL = "https://api.revenium.ai"
	apiKeyPrefix   = "hak_"
)

// Config holds the configuration for the Revenium metering middleware.
type Config struct {
	// APIKey is the Revenium API key (required, must start with "hak_").
	APIKey string

	// BaseURL is the Revenium API base URL. Defaults to "https://api.revenium.ai".
	BaseURL string

	// Squad is an optional override for the squad field. When empty, the squad
	// is auto-detected from agent IDs.
	Squad string

	// Environment identifies the deployment environment (e.g., "production", "staging").
	Environment string

	// OrganizationName identifies the organization for metering.
	OrganizationName string

	// SubscriptionID is the subscription identifier for Revenium correlation.
	SubscriptionID string

	// ProductName is the product name for Revenium correlation.
	ProductName string

	// Subscriber holds subscriber metadata (ID, email, credential) for metering.
	Subscriber *SubscriberResource

	// Debug enables debug-level logging.
	Debug bool

	// HTTPClient is an optional custom HTTP client for sending metering requests.
	HTTPClient *http.Client
}

// Option is a functional option for configuring a Meter.
type Option func(*Config)

// WithAPIKey sets the Revenium API key.
func WithAPIKey(key string) Option {
	return func(c *Config) { c.APIKey = key }
}

// WithBaseURL sets the Revenium API base URL.
func WithBaseURL(url string) Option {
	return func(c *Config) { c.BaseURL = url }
}

// WithSquad sets the squad name override.
func WithSquad(squad string) Option {
	return func(c *Config) { c.Squad = squad }
}

// WithEnvironment sets the deployment environment.
func WithEnvironment(env string) Option {
	return func(c *Config) { c.Environment = env }
}

// WithOrganizationName sets the organization name for metering.
func WithOrganizationName(name string) Option {
	return func(c *Config) { c.OrganizationName = name }
}

// WithSubscriptionID sets the subscription identifier for Revenium correlation.
func WithSubscriptionID(id string) Option {
	return func(c *Config) { c.SubscriptionID = id }
}

// WithProductName sets the product name for Revenium correlation.
func WithProductName(name string) Option {
	return func(c *Config) { c.ProductName = name }
}

// WithSubscriber sets the subscriber metadata for metering payloads.
func WithSubscriber(id, email string) Option {
	return func(c *Config) {
		c.Subscriber = &SubscriberResource{ID: id, Email: email}
	}
}

// WithSubscriberCredential sets the subscriber credential for metering payloads.
// Must be called after WithSubscriber.
func WithSubscriberCredential(name, value string) Option {
	return func(c *Config) {
		if c.Subscriber == nil {
			c.Subscriber = &SubscriberResource{}
		}
		c.Subscriber.Credential = &CredentialResource{Name: name, Value: value}
	}
}

// WithDebug enables debug-level logging.
func WithDebug(debug bool) Option {
	return func(c *Config) { c.Debug = debug }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) { c.HTTPClient = client }
}

func loadFromEnv(c *Config) {
	if v := os.Getenv("REVENIUM_API_KEY"); v != "" && c.APIKey == "" {
		c.APIKey = v
	}
	if v := os.Getenv("REVENIUM_BASE_URL"); v != "" && c.BaseURL == "" {
		c.BaseURL = v
	}
	if v := os.Getenv("REVENIUM_METERING_BASE_URL"); v != "" && c.BaseURL == "" {
		c.BaseURL = v
	}
	if v := os.Getenv("REVENIUM_ENVIRONMENT"); v != "" && c.Environment == "" {
		c.Environment = v
	}
	if v := os.Getenv("REVENIUM_SQUAD"); v != "" && c.Squad == "" {
		c.Squad = v
	}
	if v := os.Getenv("REVENIUM_ORGANIZATION_NAME"); v != "" && c.OrganizationName == "" {
		c.OrganizationName = v
	}
	if v := os.Getenv("REVENIUM_SUBSCRIPTION_ID"); v != "" && c.SubscriptionID == "" {
		c.SubscriptionID = v
	}
	if v := os.Getenv("REVENIUM_PRODUCT_NAME"); v != "" && c.ProductName == "" {
		c.ProductName = v
	}
	if c.Subscriber == nil {
		subID := os.Getenv("REVENIUM_SUBSCRIBER_ID")
		subEmail := os.Getenv("REVENIUM_SUBSCRIBER_EMAIL")
		if subID != "" || subEmail != "" {
			c.Subscriber = &SubscriberResource{ID: subID, Email: subEmail}
		}
	}
}

func (c *Config) validate() error {
	if c.APIKey == "" {
		return newConfigError("API key is required", nil)
	}
	if !strings.HasPrefix(c.APIKey, apiKeyPrefix) {
		return newConfigError("API key must start with \"hak_\"", nil)
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
}
