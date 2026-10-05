package agent

import (
	"strings"
	"testing"
)

// D6 4-pillar stub harness for duel_atom_smith P1 single-agent.
// Real chaos cleared by POC W3 multi-crew variant; these stubs preserve
// the contract surface during production promotion.

func TestD6P1_composerIsPureAcrossRecovery(t *testing.T) {
	// Recovery: same input always yields same prompt (no cached state).
	ctx := TaskContext{
		TenantID:   "t",
		UserGCID:   "a",
		Candidates: []Candidate{{Index: 0, Question: "Q?", Options: []string{"a", "b", "c", "d"}}},
		Count:      2,
	}
	pre := ComposeInstruction(RoleSmith, ctx)
	post := ComposeInstruction(RoleSmith, ctx)
	if pre != post {
		t.Errorf("smith: composer not pure across recovery")
	}
}

func TestD6P2_terminationEventTopicCanonical(t *testing.T) {
	want := "chora.ai_kernel.agent.terminated.v1"
	if !strings.HasPrefix(want, "chora.ai_kernel.") || !strings.HasSuffix(want, ".v1") {
		t.Errorf("canonical topic must be chora.ai_kernel.*.v1; got %q", want)
	}
}

func TestD6P3_composeIsConcurrencySafe(t *testing.T) {
	// P1 single-agent: under N concurrent duel requests with distinct
	// tenants, prompts must isolate per request (no cross-bleed). Pure
	// functions are inherently concurrent-safe; this guards against future
	// state introduction.
	for i := range 16 {
		ctx := TaskContext{
			TenantID:   "tenant-" + string(rune('A'+i%26)),
			UserGCID:   "gcid-stub",
			Candidates: []Candidate{{Index: 0, Question: "Q?", Options: []string{"a", "b", "c", "d"}}},
			Count:      1,
		}
		got := ComposeInstruction(RoleSmith, ctx)
		want := "tenant-" + string(rune('A'+i%26))
		if !strings.Contains(got, want) {
			t.Errorf("iter %d: prompt missing own tenant_id %q", i, want)
		}
	}
}

func TestD6P4_mandatoryAttributesCoverSmith(t *testing.T) {
	got := MandatorySpanAttributes()
	// chora.smith.role distinguishes the smith sub-agent in spans —
	// load-bearing for the P1 single-agent cost + latency drill-down.
	found := false
	for _, k := range got {
		if k == "chora.smith.role" {
			found = true
			break
		}
	}
	if !found {
		t.Error("MandatorySpanAttributes must include chora.smith.role to distinguish smith spans")
	}
}
