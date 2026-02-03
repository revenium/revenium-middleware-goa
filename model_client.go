package revenium

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"goa.design/goa-ai/runtime/agent/model"
)

const iso8601 = "2006-01-02T15:04:05Z"

// meteringClient wraps a model.Client to capture LLM completion metrics.
type meteringClient struct {
	inner          model.Client
	meter          *Meter
	modelID        string
	agentID        string
	provider       string
	capturePrompts bool
}

func (c *meteringClient) Complete(ctx context.Context, req *model.Request) (*model.Response, error) {
	start := time.Now()
	resp, err := c.inner.Complete(ctx, req)
	end := time.Now()
	elapsed := end.Sub(start)

	if err != nil {
		return nil, err
	}

	// squad := ResolveSquad(c.meter.cfg, c.agentID)
	payload := &MeteringPayload{
		Model:               c.resolveModel(req),
		InputTokenCount:     resp.Usage.InputTokens,
		OutputTokenCount:    resp.Usage.OutputTokens,
		TotalTokenCount:     resp.Usage.InputTokens + resp.Usage.OutputTokens,
		StopReason:          MapStopReason(resp.StopReason),
		RequestTime:         start.UTC().Format(iso8601),
		CompletionStartTime: start.UTC().Format(iso8601),
		ResponseTime:        end.UTC().Format(iso8601),
		RequestDuration:     elapsed.Milliseconds(),
		Provider:            c.provider,
		IsStreamed:          false,
		BillingUnit:         "PER_TOKEN",
		Agent:               c.agentID,
		// SquadID:             squad,
		// SquadName:           squad,
		CacheReadTokenCount:     resp.Usage.CacheReadTokens,
		CacheCreationTokenCount: resp.Usage.CacheWriteTokens,
	}

	if tc := GetTraceContext(ctx); tc != nil {
		payload.TraceID = tc.TraceID
		payload.TraceName = tc.TraceName
		payload.TraceType = tc.TraceType
		payload.TransactionID = tc.TransactionID
		payload.ParentTxnID = tc.ParentTxnID
		// if tc.Squad != "" {
		// 	payload.SquadID = tc.Squad
		// 	payload.SquadName = tc.Squad
		// }
	}

	if c.capturePrompts {
		populatePromptFields(payload, req, resp.Content)
	}

	c.meter.SendAsync(ctx, payload)
	return resp, nil
}

// resolveModel returns the concrete model name from the request or falls back
// to the registered model ID.
func (c *meteringClient) resolveModel(req *model.Request) string {
	if req.Model != "" {
		return req.Model
	}
	return c.modelID
}

func (c *meteringClient) Stream(ctx context.Context, req *model.Request) (model.Streamer, error) {
	start := time.Now()
	streamer, err := c.inner.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	return &meteringStreamer{
		inner:          streamer,
		meter:          c.meter,
		modelID:        c.resolveModel(req),
		agentID:        c.agentID,
		provider:       c.provider,
		capturePrompts: c.capturePrompts,
		req:            req,
		start:          start,
		ctx:            ctx,
	}, nil
}

// meteringStreamer wraps a model.Streamer to capture usage on close.
type meteringStreamer struct {
	inner          model.Streamer
	meter          *Meter
	modelID        string
	agentID        string
	provider       string
	capturePrompts bool
	req            *model.Request
	start          time.Time
	ctx            context.Context
	usage          model.TokenUsage
	stopReason     string
	responseText   strings.Builder
}

func (s *meteringStreamer) Recv() (model.Chunk, error) {
	chunk, err := s.inner.Recv()
	if chunk.UsageDelta != nil {
		s.usage.InputTokens += chunk.UsageDelta.InputTokens
		s.usage.OutputTokens += chunk.UsageDelta.OutputTokens
		s.usage.TotalTokens += chunk.UsageDelta.TotalTokens
	}
	if chunk.StopReason != "" {
		s.stopReason = chunk.StopReason
	}
	if s.capturePrompts && chunk.Message != nil {
		s.responseText.WriteString(extractMessageText(chunk.Message))
	}
	return chunk, err
}

func (s *meteringStreamer) Close() error {
	err := s.inner.Close()
	end := time.Now()
	elapsed := end.Sub(s.start)

	if s.usage.InputTokens > 0 || s.usage.OutputTokens > 0 {
		// squad := ResolveSquad(s.meter.cfg, s.agentID)
		payload := &MeteringPayload{
			Model:               s.modelID,
			InputTokenCount:     s.usage.InputTokens,
			OutputTokenCount:    s.usage.OutputTokens,
			TotalTokenCount:     s.usage.InputTokens + s.usage.OutputTokens,
			StopReason:          MapStopReason(s.stopReason),
			RequestTime:         s.start.UTC().Format(iso8601),
			CompletionStartTime: s.start.UTC().Format(iso8601),
			ResponseTime:        end.UTC().Format(iso8601),
			RequestDuration:     elapsed.Milliseconds(),
			Provider:            s.provider,
			IsStreamed:          true,
			BillingUnit:         "PER_TOKEN",
			Agent:               s.agentID,
			// SquadID:             squad,
			// SquadName:           squad,
		}

		if tc := GetTraceContext(s.ctx); tc != nil {
			payload.TraceID = tc.TraceID
			payload.TraceName = tc.TraceName
			payload.TraceType = tc.TraceType
			payload.TransactionID = tc.TransactionID
			payload.ParentTxnID = tc.ParentTxnID
			// if tc.Squad != "" {
			// 	payload.SquadID = tc.Squad
			// 	payload.SquadName = tc.Squad
			// }
		}

		if s.capturePrompts {
			populatePromptFields(payload, s.req, nil)
			payload.OutputResponse = s.responseText.String()
		}

		s.meter.SendAsync(s.ctx, payload)
	}

	return err
}

func (s *meteringStreamer) Metadata() map[string]any {
	return s.inner.Metadata()
}

// extractMessageText returns the concatenated text content of a message.
func extractMessageText(msg *model.Message) string {
	var b strings.Builder
	for _, p := range msg.Parts {
		if tp, ok := p.(model.TextPart); ok {
			b.WriteString(tp.Text)
		}
	}
	return b.String()
}

// inputMessage is a simplified representation of a conversation message
// for JSON serialization into the inputMessages payload field.
type inputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// populatePromptFields extracts prompt data from the model request and response
// and sets the corresponding fields on the metering payload.
func populatePromptFields(payload *MeteringPayload, req *model.Request, responseContent []model.Message) {
	var systemParts []string
	var inputMsgs []inputMessage

	for _, msg := range req.Messages {
		text := extractMessageText(msg)
		if text == "" {
			continue
		}
		if msg.Role == model.ConversationRoleSystem {
			systemParts = append(systemParts, text)
		} else {
			inputMsgs = append(inputMsgs, inputMessage{
				Role:    string(msg.Role),
				Content: text,
			})
		}
	}

	if len(systemParts) > 0 {
		payload.SystemPrompt = strings.Join(systemParts, "\n")
	}

	if len(inputMsgs) > 0 {
		if data, err := json.Marshal(inputMsgs); err == nil {
			payload.InputMessages = string(data)
		}
	}

	if len(responseContent) > 0 {
		var parts []string
		for i := range responseContent {
			if t := extractMessageText(&responseContent[i]); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			payload.OutputResponse = strings.Join(parts, "\n")
		}
	}
}
