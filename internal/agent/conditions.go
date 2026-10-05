package agent

import "google.golang.org/adk/session"

// conditions.go — ADR-197 M-A condition extractor for the duel_atom_smith crew.
//
// The only prompt-shaping discriminant is the role (smith); the candidates /
// tags / proficiencies are the input content (never surfaced as conditions).
// The returned function matches promptstamping.ConditionExtractor so main can
// wire it directly.
func SmithConditions(role AgentRole) func(session.ReadonlyState) map[string]string {
	return func(session.ReadonlyState) map[string]string {
		return map[string]string{"role": string(role)}
	}
}
