package revenium

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const meteringPath = "/meter/v2/ai/completions"

// MeteringPayload matches the AICompletionMetadataResource schema from the
// Revenium metering API OpenAPI spec.
type MeteringPayload struct {
	// Required fields
	Model               string `json:"model"`
	InputTokenCount     int    `json:"inputTokenCount"`
	OutputTokenCount    int    `json:"outputTokenCount"`
	TotalTokenCount     int    `json:"totalTokenCount"`
	StopReason          string `json:"stopReason"`
	RequestTime         string `json:"requestTime"`
	CompletionStartTime string `json:"completionStartTime"`
	ResponseTime        string `json:"responseTime"`
	RequestDuration     int64  `json:"requestDuration"`
	Provider            string `json:"provider"`
	IsStreamed          bool   `json:"isStreamed"`
	BillingUnit         string `json:"billingUnit"`

	// Optional fields
	TransactionID    string `json:"transactionId,omitempty"`
	TraceID          string `json:"traceId,omitempty"`
	TraceName        string `json:"traceName,omitempty"`
	TraceType        string `json:"traceType,omitempty"`
	ParentTxnID      string `json:"parentTransactionId,omitempty"`
	Agent            string `json:"agent,omitempty"`
	SquadID          string `json:"squadId,omitempty"`
	SquadName        string `json:"squadName,omitempty"`
	OrganizationName string `json:"organizationName,omitempty"`
	Environment      string `json:"environment,omitempty"`
	MiddlewareSource string `json:"middlewareSource,omitempty"`

	CacheReadTokenCount     int `json:"cacheReadTokenCount,omitempty"`
	CacheCreationTokenCount int `json:"cacheCreationTokenCount,omitempty"`

	SystemPrompt     string `json:"systemPrompt,omitempty"`
	InputMessages    string `json:"inputMessages,omitempty"`
	OutputResponse   string `json:"outputResponse,omitempty"`
	PromptsTruncated bool   `json:"promptsTruncated,omitempty"`

	SubscriptionID string              `json:"subscriptionId,omitempty"`
	ProductName    string              `json:"productName,omitempty"`
	Subscriber     *SubscriberResource `json:"subscriber,omitempty"`
}

// SubscriberResource identifies the end-user making the AI request.
type SubscriberResource struct {
	ID         string              `json:"id,omitempty"`
	Email      string              `json:"email,omitempty"`
	Credential *CredentialResource `json:"credential,omitempty"`
}

// CredentialResource identifies the API key or credential used by the subscriber.
type CredentialResource struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// Allowed stopReason values per the Revenium API.
const (
	StopReasonEnd             = "END"
	StopReasonEndSequence     = "END_SEQUENCE"
	StopReasonTimeout         = "TIMEOUT"
	StopReasonTokenLimit      = "TOKEN_LIMIT"
	StopReasonCostLimit       = "COST_LIMIT"
	StopReasonCompletionLimit = "COMPLETION_LIMIT"
	StopReasonError           = "ERROR"
	StopReasonCancelled       = "CANCELLED"
)

// MapStopReason maps provider-specific stop reasons to Revenium's enum values.
func MapStopReason(providerReason string) string {
	switch providerReason {
	case "stop", "end_turn", "complete":
		return StopReasonEnd
	case "tool_calls", "tool_use":
		return StopReasonEnd
	case "length", "max_tokens":
		return StopReasonTokenLimit
	case "content_filter":
		return StopReasonEnd
	case "":
		return StopReasonEnd
	default:
		return StopReasonEnd
	}
}

// Meter is the core metering client that sends payloads to the Revenium API.
type Meter struct {
	cfg    *Config
	logger *Logger
	wg     sync.WaitGroup
	traces sync.Map // runID → traceID for cross-agent trace correlation
}

// RegisterTrace stores the traceID associated with a run so child runs can
// inherit the same trace.
func (m *Meter) RegisterTrace(runID, traceID string) {
	m.traces.Store(runID, traceID)
	m.logger.Debug("registered trace: run=%s trace=%s", runID, traceID)
}

// LookupTrace retrieves the traceID previously registered for runID.
func (m *Meter) LookupTrace(runID string) (string, bool) {
	v, ok := m.traces.Load(runID)
	if !ok {
		return "", false
	}
	return v.(string), true
}

// UnregisterTrace removes the trace mapping for a completed run.
func (m *Meter) UnregisterTrace(runID string) {
	m.traces.Delete(runID)
	m.logger.Debug("unregistered trace: run=%s", runID)
}

// NewMeter creates a new Meter with the given options.
func NewMeter(opts ...Option) (*Meter, error) {
	cfg := &Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	loadFromEnv(cfg)
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Meter{
		cfg:    cfg,
		logger: newLogger(cfg.Debug),
	}, nil
}

// SendAsync sends a metering payload asynchronously. It never blocks the caller.
// The send uses a detached context so it is not canceled when the caller's
// context ends (e.g., when the runtime moves to the next planning phase).
func (m *Meter) SendAsync(_ context.Context, payload *MeteringPayload) {
	payload.MiddlewareSource = middlewareSource
	if payload.Environment == "" {
		payload.Environment = m.cfg.Environment
	}
	if payload.OrganizationName == "" {
		payload.OrganizationName = m.cfg.OrganizationName
	}
	if payload.SubscriptionID == "" {
		payload.SubscriptionID = m.cfg.SubscriptionID
	}
	if payload.ProductName == "" {
		payload.ProductName = m.cfg.ProductName
	}
	if payload.Subscriber == nil && m.cfg.Subscriber != nil {
		payload.Subscriber = m.cfg.Subscriber
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		// Use a detached context with a generous timeout so metering is not
		// canceled when the caller's request context ends.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.sendWithRetry(ctx, payload); err != nil {
			m.logger.Error("failed to send metering payload: %v", err)
		}
	}()
}

// Flush waits for all pending async sends to complete.
func (m *Meter) Flush() {
	m.wg.Wait()
}

func (m *Meter) sendWithRetry(ctx context.Context, payload *MeteringPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return newMeteringError("failed to marshal payload", err)
	}

	m.logger.Debug("metering payload: %s", string(body))

	url := m.cfg.BaseURL + meteringPath
	backoff := time.Second

	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			m.logger.Debug("retrying metering request (attempt %d/%d)", attempt, maxRetries)
			select {
			case <-ctx.Done():
				return newNetworkError("context canceled during retry", ctx.Err())
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		err = m.send(ctx, url, body)
		if err == nil {
			m.logger.Debug("metering payload sent successfully (model=%s, tokens=%d+%d)",
				payload.Model, payload.InputTokenCount, payload.OutputTokenCount)
			return nil
		}
		m.logger.Warn("metering request failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
	}
	return err
}

func (m *Meter) send(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return newNetworkError("failed to create request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.cfg.APIKey)
	req.Header.Set("User-Agent", userAgent)

	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return newNetworkError("request failed", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	m.logger.Debug("metering API response (%d): %s", resp.StatusCode, string(respBody))
	return newMeteringError(fmt.Sprintf("unexpected status code: %d", resp.StatusCode), nil)
}
