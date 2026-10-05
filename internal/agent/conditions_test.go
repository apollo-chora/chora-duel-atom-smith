package agent

// RED-first test for the ADR-197 M-A condition extractor for the
// duel_atom_smith crew. The only prompt discriminant is the role (smith) —
// the candidates / tags / proficiencies are input content, NOT discriminants.

import "testing"

func TestSmithConditions_role(t *testing.T) {
	if c := SmithConditions(RoleSmith)(nil); c["role"] != "smith" {
		t.Errorf("smith role: got %q", c["role"])
	}
}
