package agent

import (
	"errors"
	"testing"
)

// fakeState is a minimal StateReader for testing TaskContextFromState without
// the ADK runtime. Get returns ErrAbsent for keys not in the map (mirroring
// the ADK session store's key-absent error).
type fakeState struct{ m map[string]any }

var errAbsent = errors.New("absent")

func (f fakeState) Get(key string) (any, error) {
	if v, ok := f.m[key]; ok {
		return v, nil
	}
	return nil, errAbsent
}

func TestTaskContextFromState_readsAllKeys(t *testing.T) {
	st := fakeState{m: map[string]any{
		"tenant_id":          "11111111-1111-7111-8111-111111111111",
		"user_gcid":          "00000000-0000-7000-8000-000000001999",
		"candidates_json":    `[{"index":0,"question":"Q?","options":["a","b","c","d"]}]`,
		"shared_tags_json":   `["math","science"]`,
		"proficiencies_json": `[1100,1300]`,
		"profiles_json":      `{"gcid-1":"intermediate"}`,
		"count":              3,
	}}
	tc := TaskContextFromState(st)
	if tc.TenantID != "11111111-1111-7111-8111-111111111111" {
		t.Errorf("TenantID = %q", tc.TenantID)
	}
	if tc.UserGCID != "00000000-0000-7000-8000-000000001999" {
		t.Errorf("UserGCID = %q", tc.UserGCID)
	}
	if tc.Count != 3 {
		t.Errorf("Count = %d; want 3", tc.Count)
	}
	if len(tc.SharedTags) != 2 || tc.SharedTags[0] != "math" || tc.SharedTags[1] != "science" {
		t.Errorf("SharedTags = %v; want [math science]", tc.SharedTags)
	}
	if len(tc.Proficiencies) != 2 || tc.Proficiencies[0] != 1100 || tc.Proficiencies[1] != 1300 {
		t.Errorf("Proficiencies = %v; want [1100 1300]", tc.Proficiencies)
	}
	if len(tc.Profiles) != 1 || tc.Profiles["gcid-1"] != "intermediate" {
		t.Errorf("Profiles = %v; want {gcid-1: intermediate}", tc.Profiles)
	}
}

func TestTaskContextFromState_missingKeysAreEmpty(t *testing.T) {
	tc := TaskContextFromState(fakeState{m: map[string]any{}})
	if tc.TenantID != "" || tc.UserGCID != "" || tc.Count != 0 {
		t.Errorf("missing keys should yield empty fields; got %+v", tc)
	}
	if tc.Candidates != nil || tc.SharedTags != nil || tc.Proficiencies != nil || tc.Profiles != nil {
		t.Errorf("missing keys should yield nil slices/maps; got %+v", tc)
	}
}

func TestTaskContextFromState_nilStateIsEmpty(t *testing.T) {
	tc := TaskContextFromState(nil)
	if tc.TenantID != "" || tc.UserGCID != "" || tc.Count != 0 {
		t.Errorf("nil state should yield zero TaskContext; got %+v", tc)
	}
	if tc.Candidates != nil || tc.SharedTags != nil || tc.Proficiencies != nil || tc.Profiles != nil {
		t.Errorf("nil state should yield nil slices/maps; got %+v", tc)
	}
}

func TestTaskContextFromState_candidatesJSON_parsed(t *testing.T) {
	st := fakeState{m: map[string]any{
		"candidates_json": `[{"index":0,"question":"What is 2+2?","options":["3","4","5","6"]},{"index":1,"question":"Capital?","options":["London","Paris","Berlin","Madrid"]}]`,
	}}
	tc := TaskContextFromState(st)
	if len(tc.Candidates) != 2 {
		t.Fatalf("Candidates len = %d; want 2", len(tc.Candidates))
	}
	if tc.Candidates[0].Index != 0 || tc.Candidates[0].Question != "What is 2+2?" {
		t.Errorf("Candidates[0] = %+v", tc.Candidates[0])
	}
	if len(tc.Candidates[0].Options) != 4 || tc.Candidates[0].Options[1] != "4" {
		t.Errorf("Candidates[0].Options = %v", tc.Candidates[0].Options)
	}
	if tc.Candidates[1].Index != 1 || tc.Candidates[1].Question != "Capital?" {
		t.Errorf("Candidates[1] = %+v", tc.Candidates[1])
	}
}

func TestComposeInstruction_fromState_carriesCandidates(t *testing.T) {
	st := fakeState{m: map[string]any{
		"candidates_json": `[{"index":0,"question":"specific-marker-7f3a2c","options":["a","b","c","d"]}]`,
		"count":           1,
	}}
	got := ComposeInstruction(RoleSmith, TaskContextFromState(st))
	if !contains(got, "specific-marker-7f3a2c") {
		t.Errorf("smith instruction must contain the candidate question from state")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
