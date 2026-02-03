// Package revenium provides metering middleware for the goa-ai agent framework.
//
// It captures LLM completion metrics (tokens, timing, model) and agent activity
// (tool calls, workflow phases) and sends them asynchronously to the Revenium
// metering API.
//
// Two integration points are provided:
//
//   - MeteringPlanner wraps planner.Planner to intercept LLM calls via a
//     decorated model.Client.
//   - MeteringSink wraps stream.Sink to observe tool and workflow events.
//
// Usage:
//
//	meter, err := revenium.NewMeter(
//	    revenium.WithAPIKey(os.Getenv("REVENIUM_API_KEY")),
//	    revenium.WithEnvironment("production"),
//	)
//	if err != nil {
//	    panic(err)
//	}
//	defer meter.Flush()
//
//	// Wrap planner
//	cfg := assistant.AssistantAgentConfig{
//	    Planner: &revenium.MeteringPlanner{
//	        Inner:   myPlanner,
//	        Meter:   meter,
//	        AgentID: "demo.assistant",
//	    },
//	}
//
//	// Wrap sink
//	rt := runtime.New(runtime.WithStream(&revenium.MeteringSink{
//	    Inner: &ConsoleSink{},
//	    Meter: meter,
//	}))
package revenium
