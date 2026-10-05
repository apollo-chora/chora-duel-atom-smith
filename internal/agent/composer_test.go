package agent

import (
	"strings"
	"testing"
)

func TestComposeSmith_emitsSixCreateBlocksInOrder(t *testing.T) {
	ctx := TaskContext{
		TenantID:   "tenant-x",
		UserGCID:   "gcid-y",
		Candidates: []Candidate{{Index: 0, Question: "What is 2+2?", Options: []string{"3", "4", "5", "6"}}},
		Count:      3,
	}
	got := ComposeInstruction(RoleSmith, ctx)
	assertCreateBlocksOrdered(t, "smith", got)
}

func assertCreateBlocksOrdered(t *testing.T, role, got string) {
	t.Helper()
	blocks := []string{
		"[CONTEXT]",
		"[ROLE]",
		"[EXAMPLES]",
		"[AUDIENCE]",
		"[TASK]",
		"[EXPECTED OUTPUT]",
	}
	lastIdx := -1
	for _, b := range blocks {
		i := strings.Index(got, b)
		if i < 0 {
			t.Errorf("%s: CREATE block %q missing", role, b)
			continue
		}
		if i <= lastIdx {
			t.Errorf("%s: %q at %d after %d (out of order)", role, b, i, lastIdx)
		}
		lastIdx = i
	}
}

func TestCompose_isDeterministic(t *testing.T) {
	ctx := TaskContext{
		TenantID:   "t",
		UserGCID:   "a",
		Candidates: []Candidate{{Index: 0, Question: "Q?", Options: []string{"a", "b", "c", "d"}}},
		Count:      2,
	}
	x := ComposeInstruction(RoleSmith, ctx)
	y := ComposeInstruction(RoleSmith, ctx)
	if x != y {
		t.Errorf("smith: not deterministic (IMDA D2 break)")
	}
}

func TestCompose_candidatesTableRenders(t *testing.T) {
	ctx := TaskContext{
		TenantID: "t",
		UserGCID: "a",
		Candidates: []Candidate{
			{Index: 0, Question: "What is 2+2?", Options: []string{"3", "4", "5", "6"}},
			{Index: 1, Question: "Capital of France?", Options: []string{"London", "Paris", "Berlin", "Madrid"}},
		},
		Count: 2,
	}
	got := ComposeInstruction(RoleSmith, ctx)
	if !strings.Contains(got, "[0]") {
		t.Error("candidate index 0 must appear in prompt")
	}
	if !strings.Contains(got, "What is 2+2?") {
		t.Error("candidate 0 question must appear in prompt")
	}
	if !strings.Contains(got, "[1]") {
		t.Error("candidate index 1 must appear in prompt")
	}
	if !strings.Contains(got, "Capital of France?") {
		t.Error("candidate 1 question must appear in prompt")
	}
	// Options should be rendered (pipe-separated).
	if !strings.Contains(got, "3 | 4 | 5 | 6") {
		t.Error("candidate 0 options must appear pipe-separated in prompt")
	}
}

func TestCompose_emptyCandidates_generateAllInstruction(t *testing.T) {
	ctx := TaskContext{
		TenantID:   "t",
		UserGCID:   "a",
		Candidates: nil,
		Count:      3,
	}
	got := ComposeInstruction(RoleSmith, ctx)
	if !strings.Contains(got, "generate ALL") {
		t.Error("empty candidates must trigger the generate-all instruction")
	}
}

func TestCompose_difficultyLabelInterpolation(t *testing.T) {
	tests := []struct {
		name string
		prof []int
		want string
	}{
		{"beginner", []int{900}, "beginner"},
		{"intermediate", []int{1200}, "intermediate"},
		{"advanced", []int{1500}, "advanced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := TaskContext{
				TenantID:      "t",
				UserGCID:      "a",
				Proficiencies: tt.prof,
				Count:         1,
			}
			got := ComposeInstruction(RoleSmith, ctx)
			if !strings.Contains(got, tt.want) {
				t.Errorf("difficulty %q must appear in prompt; got prompt without it", tt.want)
			}
		})
	}
}

func TestCompose_sharedTagsSurface(t *testing.T) {
	ctx := TaskContext{
		TenantID:   "t",
		UserGCID:   "a",
		SharedTags: []string{"mathematics", "geometry"},
		Count:      1,
	}
	got := ComposeInstruction(RoleSmith, ctx)
	if !strings.Contains(got, "mathematics") {
		t.Error("shared tag 'mathematics' must appear in prompt")
	}
	if !strings.Contains(got, "geometry") {
		t.Error("shared tag 'geometry' must appear in prompt")
	}
}

func TestCompose_expectedOutputContract(t *testing.T) {
	ctx := TaskContext{TenantID: "t", UserGCID: "a", Count: 1}
	got := ComposeInstruction(RoleSmith, ctx)
	if !strings.Contains(got, `"picks"`) {
		t.Error("EXPECTED OUTPUT must declare the picks field")
	}
	if !strings.Contains(got, `"generated"`) {
		t.Error("EXPECTED OUTPUT must declare the generated field")
	}
	if !strings.Contains(got, `"question"`) {
		t.Error("EXPECTED OUTPUT must declare the question field in generated")
	}
	if !strings.Contains(got, `"options"`) {
		t.Error("EXPECTED OUTPUT must declare the options field in generated")
	}
	if !strings.Contains(got, `"correct_answer"`) {
		t.Error("EXPECTED OUTPUT must declare the correct_answer field in generated")
	}
}

func TestCompose_webResearchMentioned(t *testing.T) {
	ctx := TaskContext{TenantID: "t", UserGCID: "a", Count: 1}
	got := ComposeInstruction(RoleSmith, ctx)
	if !strings.Contains(got, "web_research") {
		t.Error("TASK block must mention the web_research tool")
	}
}

func TestCompose_emptyContext_minimalValidInstruction(t *testing.T) {
	// Empty context must still emit all 6 blocks without panicking.
	ctx := TaskContext{}
	got := ComposeInstruction(RoleSmith, ctx)
	assertCreateBlocksOrdered(t, "smith-empty", got)
}

func TestMandatorySpanAttributes_coversSmith(t *testing.T) {
	required := []string{
		"chora.tenant_id",
		"chora.user_gcid",
		"chora.mana_tier",
		"chora.crew_kind",
		"chora.smith.role",
		"gen_ai.request.model",
		"gen_ai.usage.output_tokens",
	}
	got := MandatorySpanAttributes()
	seen := make(map[string]struct{}, len(got))
	for _, k := range got {
		seen[k] = struct{}{}
	}
	for _, r := range required {
		if _, ok := seen[r]; !ok {
			t.Errorf("MandatorySpanAttributes missing %q", r)
		}
	}
}

func TestMandatorySpanAttributes_noDuplicates(t *testing.T) {
	got := MandatorySpanAttributes()
	seen := make(map[string]struct{}, len(got))
	for _, k := range got {
		if _, dup := seen[k]; dup {
			t.Errorf("duplicate attribute %q", k)
		}
		seen[k] = struct{}{}
	}
}
