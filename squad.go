package revenium

import "strings"

// DetectSquad extracts the service prefix from an agent ID.
// For example, "demo.assistant" returns "demo".
func DetectSquad(agentID string) string {
	if i := strings.Index(agentID, "."); i > 0 {
		return agentID[:i]
	}
	return agentID
}

// CompositeSquadName joins multiple service prefixes into a composite squad name.
func CompositeSquadName(squads ...string) string {
	return strings.Join(squads, "+")
}

// ResolveSquad returns the configured squad override, or auto-detects from the agent ID.
func ResolveSquad(cfg *Config, agentID string) string {
	if cfg.Squad != "" {
		return cfg.Squad
	}
	return DetectSquad(agentID)
}
