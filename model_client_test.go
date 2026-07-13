package revenium

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"goa.design/goa-ai/runtime/agent/model"
)

// completedStreamer models a provider stream after clean EOF.
type completedStreamer struct {
	response *model.Response
	closed   bool
}

// TestMeteringStreamerUsesCanonicalResponse verifies that stream metering reads
// usage, stop reason, and output from the provider's completed response instead
// of reconstructing them from incremental chunks.
func TestMeteringStreamerUsesCanonicalResponse(t *testing.T) {
	t.Parallel()

	payloads := make(chan MeteringPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload MeteringPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode metering payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payloads <- payload
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	meter, err := NewMeter(
		WithAPIKey("hak_test"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("create meter: %v", err)
	}

	response := &model.Response{
		Content: []model.Message{{
			Role: model.ConversationRoleAssistant,
			Parts: []model.Part{
				model.ThinkingPart{Text: "reasoning", Final: true},
				model.TextPart{Text: "canonical answer"},
			},
		}},
		Usage: model.TokenUsage{
			InputTokens:      11,
			OutputTokens:     7,
			TotalTokens:      23,
			CacheReadTokens:  3,
			CacheWriteTokens: 2,
			Model:            "provider-model",
		},
		StopReason: "stop",
	}
	inner := &completedStreamer{response: response}
	streamer := &meteringStreamer{
		inner:          inner,
		meter:          meter,
		modelID:        "configured-model",
		agentID:        "test-agent",
		provider:       "test-provider",
		capturePrompts: true,
		req: &model.Request{
			Messages: []*model.Message{{
				Role:  model.ConversationRoleUser,
				Parts: []model.Part{model.TextPart{Text: "question"}},
			}},
		},
		ctx: context.Background(),
	}

	if got := streamer.Response(); got != response {
		t.Fatal("Response did not forward the provider response")
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	meter.Flush()

	payload := <-payloads
	if payload.Model != "provider-model" {
		t.Errorf("Model = %q, want provider-model", payload.Model)
	}
	if payload.InputTokenCount != 11 || payload.OutputTokenCount != 7 || payload.TotalTokenCount != 23 {
		t.Errorf(
			"token counts = %d/%d/%d, want 11/7/23",
			payload.InputTokenCount,
			payload.OutputTokenCount,
			payload.TotalTokenCount,
		)
	}
	if payload.CacheReadTokenCount != 3 || payload.CacheCreationTokenCount != 2 {
		t.Errorf(
			"cache token counts = %d/%d, want 3/2",
			payload.CacheReadTokenCount,
			payload.CacheCreationTokenCount,
		)
	}
	if payload.StopReason != MapStopReason(response.StopReason) {
		t.Errorf("StopReason = %q, want %q", payload.StopReason, MapStopReason(response.StopReason))
	}
	if payload.OutputResponse != "canonical answer" {
		t.Errorf("OutputResponse = %q, want canonical answer", payload.OutputResponse)
	}
	if !inner.closed {
		t.Fatal("inner stream was not closed")
	}
}

func (s *completedStreamer) Recv() (model.Chunk, error) {
	return nil, io.EOF
}

func (s *completedStreamer) Close() error {
	s.closed = true
	return nil
}

func (s *completedStreamer) Metadata() map[string]any {
	return nil
}

func (s *completedStreamer) Response() *model.Response {
	return s.response
}
