# Revenium Metering Middleware for goa-ai

Metering middleware for the [goa-ai](https://goa.design/goa-ai/) agent framework. Captures LLM completion metrics (tokens, timing, model) and agent activity (tool calls, workflow phases) and sends them asynchronously to the Revenium metering API.

## Installation

```bash
go get github.com/revenium/revenium-middleware-goa
```

Then import it in your Go code:

```go
import (
    revenium "github.com/revenium/revenium-middleware-goa"
)
```

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `REVENIUM_API_KEY` | Yes | Revenium API key (must start with `hak_`) |
| `REVENIUM_METERING_BASE_URL` | No | Revenium metering API base URL (default: `https://api.revenium.ai`) |
| `REVENIUM_BASE_URL` | No | Alias for `REVENIUM_METERING_BASE_URL` |
| `REVENIUM_ENVIRONMENT` | No | Deployment environment (e.g., `production`, `staging`) |
| `REVENIUM_SQUAD` | No | Override for the squad/service-group identifier |
| `REVENIUM_SUBSCRIPTION_ID` | No | Subscription identifier for Revenium correlation |
| `REVENIUM_PRODUCT_NAME` | No | Product name for Revenium correlation |
| `REVENIUM_SUBSCRIBER_ID` | No | Subscriber/end-user identifier |
| `REVENIUM_SUBSCRIBER_EMAIL` | No | Subscriber email address |

When both `REVENIUM_BASE_URL` and `REVENIUM_METERING_BASE_URL` are set, `REVENIUM_BASE_URL` takes precedence. Programmatic options always override environment variables.

## Quick Start

```bash
export REVENIUM_API_KEY="hak_your_key_here"
export REVENIUM_METERING_BASE_URL="https://your-revenium-instance.example.com"
export OPENAI_API_KEY="sk-..."
go run ./main.go
```

## Usage

### 1. Create a Meter

```go
import revenium "github.com/revenium/revenium-middleware-goa"

meter, err := revenium.NewMeter(
    revenium.WithAPIKey(os.Getenv("REVENIUM_API_KEY")),
    revenium.WithEnvironment("production"),
)
if err != nil {
    panic(err)
}
defer meter.Flush()
```

If `REVENIUM_API_KEY` is set in the environment, you can omit the `WithAPIKey` option — the middleware reads it automatically.

You can also set the base URL and subscriber metadata programmatically:

```go
meter, err := revenium.NewMeter(
    revenium.WithBaseURL("https://your-revenium-instance.example.com"),
    revenium.WithSubscriptionID("sub-456"),
    revenium.WithProductName("Free Trial"),
    revenium.WithSubscriber("user-123", "user@example.com"),
    revenium.WithSubscriberCredential("Production Key", "pk-abc123"),
)
```

### 2. Wrap Planners with MeteringPlanner

Wrap each agent's planner to intercept LLM calls and capture token usage, timing, and model metadata:

```go
cfg := assistant.AssistantAgentConfig{
    Planner: &revenium.MeteringPlanner{
        Inner: &LLMPlanner{
            systemPrompt: "You are a helpful assistant.",
            modelID:      "openai",
            toolSpecs:    assistantspecs.Specs,
        },
        Meter:   meter,
        AgentID: "demo.assistant",
    },
}

err = assistant.RegisterAssistantAgent(ctx, rt, cfg)
```

The `AgentID` is used for squad auto-detection (the prefix before the first `.` becomes the squad name). For example, `"demo.assistant"` produces squad `"demo"`. Override this with `WithSquad` or `REVENIUM_SQUAD`.

To capture system prompts, input messages, and output responses in metering payloads, set `CapturePrompts: true`:

```go
Planner: &revenium.MeteringPlanner{
    Inner:          &LLMPlanner{...},
    Meter:          meter,
    AgentID:        "demo.assistant",
    CapturePrompts: true,
},
```

This populates the `systemPrompt`, `inputMessages`, and `outputResponse` fields on each metering payload. Disabled by default since prompts may contain sensitive data.

### 3. Wrap the Stream Sink with MeteringSink

Wrap the stream sink to observe tool calls, workflow phases, and child agent runs:

```go
rt := runtime.New(
    runtime.WithStream(&revenium.MeteringSink{
        Inner: &ConsoleSink{},
        Meter: meter,
    }),
)
```

### Full Example

See `main.go` for a complete working example with both integration points wired in.

## What Gets Metered

### MeteringPlanner (LLM completions)

Each `model.Client.Complete()` and `model.Client.Stream()` call sends a payload to `POST {baseURL}/meter/v2/ai/completions` containing:

- **model** — Model identifier (e.g., `"openai"`)
- **requestTokens** / **responseTokens** — Input and output token counts
- **responseTime** — Wall-clock latency in milliseconds
- **traceId** — Correlation ID across the full request
- **squad** — Agent group identifier (auto-detected or configured)
- **environment** — Deployment metadata

### MeteringSink (observability events)

Logs tool start/end, workflow phase transitions, child run links, and usage events at debug level. Enable debug logging with `WithDebug(true)` or inspect events in your own sink.

## Agent Interaction Tracking

When agents call other agents (e.g., `demo.assistant` delegates to `weather.forecaster`), the middleware automatically correlates all metering payloads across the agent chain:

- **`traceId`** — Shared across all agents in the same interaction. The top-level agent generates the traceId, and child agents inherit it.
- **`transactionId`** — Unique per agent run (the runtime's `RunID`).
- **`parentTransactionId`** — Set on child agent payloads, pointing to the parent agent's `transactionId`.

For a query like "What's the weather in Paris?":

| Agent | traceId | transactionId | parentTransactionId |
|---|---|---|---|
| demo.assistant (PlanStart) | `abc-123` | `run-parent` | _(empty)_ |
| weather.forecaster (PlanStart) | `abc-123` | `run-child` | `run-parent` |
| demo.assistant (PlanResume) | `abc-123` | `run-parent` | _(empty)_ |

This is handled automatically by `MeteringPlanner` and `MeteringSink`:

1. **MeteringPlanner** checks `run.Context.ParentRunID` — top-level runs generate a new traceId; child runs look up the parent's traceId from the meter's trace registry.
2. **MeteringSink** pre-registers child trace links on `ChildRunLinked` events (before the child planner runs) and cleans up trace mappings on terminal workflow phases.

No additional configuration is required — multi-agent trace correlation works out of the box when both agents use `MeteringPlanner` and share the same `Meter` instance.

## Configuration Precedence

1. Programmatic options (`WithAPIKey`, `WithBaseURL`, etc.)
2. Environment variables (`REVENIUM_API_KEY`, `REVENIUM_METERING_BASE_URL`, etc.)
3. Defaults (`https://api.revenium.ai`)

## Design

- **Fire-and-forget async** — Metering never blocks agent execution
- **WaitGroup flush** — `defer meter.Flush()` ensures all metering completes before exit
- **3 retries with exponential backoff** — 1s, 2s, 4s between attempts
- **PlannerContext wrapping** — Intercepts `ModelClient()` to inject metering transparently
- **Trace propagation** — TraceID flows through `context.Context` across agent boundaries
- **Standalone package** — All code under `revenium/` with no imports from `gen/` or main
