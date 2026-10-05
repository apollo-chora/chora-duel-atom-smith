// instruction_provider.go — per-turn InstructionProvider wiring for the
// duel_atom_smith crew (ADR-169 migration; runtime instruction composition).
//
// The smith composer interpolates candidates / shared tags / proficiencies /
// profiles / count from a TaskContext. At boot those values are unknown, so the
// agent must recompose its instruction PER TURN from the session state the
// chora-sharing caller populated at async_create_session
// (candidates_json / shared_tags_json / proficiencies_json / profiles_json /
// count / tenant_id / user_gcid). llmagent.Config.InstructionProvider takes
// precedence over the static Instruction field, so wiring this ensures the
// model receives the real per-duel context.
package agent

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
)

// NewInstructionProvider returns an llmagent.InstructionProvider that composes
// the role's instruction from the live session state on each turn. Wire it into
// llmagent.Config.InstructionProvider for the smith sub-agent.
func NewInstructionProvider(role AgentRole) llmagent.InstructionProvider {
	return func(rc agent.ReadonlyContext) (string, error) {
		var tc TaskContext
		if rc != nil {
			if st := rc.ReadonlyState(); st != nil {
				tc = TaskContextFromState(st)
			}
		}
		return ComposeInstruction(role, tc), nil
	}
}
