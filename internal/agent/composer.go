// Package agent holds the duel_atom_smith P1 single-agent crew's
// CREATE-pattern composer + D6 attribute contract.
//
// P1 single-agent pattern per crew-composition SKILL: one `smith` sub-agent
// that picks duel atoms from the shared-atom candidate pool (passed in session
// state by chora-sharing — agents have NO DB access, cross-DB forbidden) and
// generates fresh ephemeral MCQ atoms for any shortfall, with a `web_research`
// tool (gateway GroundedSearch RPC, ADR-231) when candidates/knowledge are
// insufficient.
//
// The composer is a PURE function (no state, no side effects) — IMDA D2
// transparency. Same input always yields the same prompt.
package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Candidate is one shared-atom projection candidate passed in session state
// by chora-sharing's DuelAtomSmithSelector. The agent picks from these by
// index.
type Candidate struct {
	Index    int      `json:"index"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// TaskContext is the per-call duel-atom-smith context.
type TaskContext struct {
	TenantID      string
	UserGCID      string
	Candidates    []Candidate
	SharedTags    []string
	Proficiencies []int
	Profiles      map[string]string
	Count         int
}

// AgentRole identifies which composer to use. duel_atom_smith is P1
// single-agent, so there is only RoleSmith.
type AgentRole string

const (
	RoleSmith AgentRole = "smith"
)

// StateReader is the minimal readonly session-state seam used to build a
// per-turn TaskContext. It matches the ADK ReadonlyState contract
// (Get(key) → (value, error); error when the key is absent), so an
// llmagent.InstructionProvider can pass `rc.ReadonlyState()` directly while
// tests substitute a fake.
type StateReader interface {
	Get(key string) (any, error)
}

// TaskContextFromState builds a per-call TaskContext from the session state the
// caller populated at async_create_session (candidates_json / shared_tags_json
// / proficiencies_json / profiles_json / count / tenant_id / user_gcid).
// Missing / wrong-typed keys yield empty fields (safe fallback handled
// downstream by safe()).
func TaskContextFromState(state StateReader) TaskContext {
	if state == nil {
		return TaskContext{}
	}
	return TaskContext{
		TenantID:      stateString(state, "tenant_id"),
		UserGCID:      stateString(state, "user_gcid"),
		Candidates:    stateCandidates(state, "candidates_json"),
		SharedTags:    stateStringSlice(state, "shared_tags_json"),
		Proficiencies: stateIntSlice(state, "proficiencies_json"),
		Profiles:      stateStringMap(state, "profiles_json"),
		Count:         stateInt(state, "count"),
	}
}

// stateString reads a string value from session state; absent / wrong-typed
// keys return "".
func stateString(state StateReader, key string) string {
	raw, err := state.Get(key)
	if err != nil {
		return ""
	}
	s, _ := raw.(string)
	return s
}

// stateInt reads an int value from session state; absent / wrong-typed keys
// return 0. Tolerates both int and float64 (JSON round-trip produces float64).
func stateInt(state StateReader, key string) int {
	raw, err := state.Get(key)
	if err != nil {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return 0
}

// stateCandidates decodes the candidates_json array from session state.
// Absent / malformed ⇒ nil (safe fallback).
func stateCandidates(state StateReader, key string) []Candidate {
	raw, err := state.Get(key)
	if err != nil {
		return nil
	}
	var jsonBytes []byte
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		jsonBytes = []byte(v)
	case []Candidate:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		jsonBytes = b
	}
	var out []Candidate
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil
	}
	return out
}

// stateStringSlice decodes a JSON array of strings from session state.
// Absent / malformed ⇒ nil.
func stateStringSlice(state StateReader, key string) []string {
	raw, err := state.Get(key)
	if err != nil {
		return nil
	}
	var jsonBytes []byte
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		jsonBytes = []byte(v)
	case []string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		jsonBytes = b
	}
	var out []string
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil
	}
	return out
}

// stateIntSlice decodes a JSON array of ints from session state.
// Absent / malformed ⇒ nil.
func stateIntSlice(state StateReader, key string) []int {
	raw, err := state.Get(key)
	if err != nil {
		return nil
	}
	var jsonBytes []byte
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		jsonBytes = []byte(v)
	case []int:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		jsonBytes = b
	}
	var out []int
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil
	}
	return out
}

// stateStringMap decodes a JSON map[string]string from session state.
// Absent / malformed ⇒ nil.
func stateStringMap(state StateReader, key string) map[string]string {
	raw, err := state.Get(key)
	if err != nil {
		return nil
	}
	var jsonBytes []byte
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		jsonBytes = []byte(v)
	case map[string]string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		jsonBytes = b
	}
	var out map[string]string
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil
	}
	return out
}

// ComposeInstruction emits the deterministic 6-block CREATE prompt for
// the specified role + task context. Pure function (IMDA D2 transparency).
func ComposeInstruction(role AgentRole, ctx TaskContext) string {
	return composeSmith(ctx)
}

// difficultyLabel maps an average proficiency (ELO-style) to a difficulty
// label: beginner (<1100), intermediate (1100-1300), advanced (>1300).
func difficultyLabel(avgProf int) string {
	switch {
	case avgProf < 1100:
		return "beginner"
	case avgProf <= 1300:
		return "intermediate"
	default:
		return "advanced"
	}
}

// avgProficiency computes the average of the proficiency values, returning
// 1200 (the default ELO) when empty.
func avgProficiency(profs []int) int {
	if len(profs) == 0 {
		return 1200
	}
	sum := 0
	for _, p := range profs {
		sum += p
	}
	return sum / len(profs)
}

func composeSmith(ctx TaskContext) string {
	var b strings.Builder

	b.WriteString("## [CONTEXT]\n")
	b.WriteString("You are running inside the Chora learning platform as the Smith " +
		"sub-agent of the duel_atom_smith crew (P1 single-agent). You pick duel atoms " +
		"from the shared-atom candidate pool (passed below) and generate fresh MCQ atoms " +
		"for any shortfall, personalised to both players' interests (shared tags + " +
		"individual tags from profiles) + proficiency.\n")
	fmt.Fprintf(&b, "Tenant: %s. User GCID: %s.\n\n",
		safe(ctx.TenantID, "<unset>"), safe(ctx.UserGCID, "<unset>"))

	// [ROLE]
	b.WriteString("## [ROLE]\n")
	b.WriteString("You are the Smith — the single agent that assembles a duel round. " +
		"You choose the best candidate atoms from the shared pool AND generate fresh " +
		"MCQs for any shortfall. You have a `web_research` tool for when the candidates " +
		"don't cover the players' interests or when a question's facts need verification. " +
		"If the tool fails, proceed without web data — never block the duel on a tool error.\n\n")

	// [EXAMPLES]
	b.WriteString("## [EXAMPLES]\n")
	b.WriteString("Example (pick + generate): 5 candidates, Count=3, shared tags match " +
		"candidates 0 and 2 → {\"picks\":[0,2],\"generated\":[{\"question\":\"...\",\"options\":" +
		"[\"a\",\"b\",\"c\",\"d\"],\"correct_answer\":\"b\"}]}\n")
	b.WriteString("Example (generate all): 0 candidates, Count=2 → {\"picks\":[]," +
		"\"generated\":[{...},{...}]}\n")
	b.WriteString("Example (pick all): 4 candidates, Count=4, all match tags → " +
		"{\"picks\":[0,1,2,3],\"generated\":[]}\n\n")

	// [AUDIENCE]
	b.WriteString("## [AUDIENCE]\n")
	b.WriteString("Your output is consumed by the chora-sharing service's " +
		"DuelAtomSmithSelector, which validates picks (in-range, deduped) and generated " +
		"atoms (4 options, correct_answer matches an option) before embedding them into " +
		"duel_rounds. Invalid entries are silently dropped — get the contract right.\n\n")

	// [TASK]
	b.WriteString("## [TASK]\n")
	fmt.Fprintf(&b, "Target atom count: %d.\n", ctx.Count)
	diff := difficultyLabel(avgProficiency(ctx.Proficiencies))
	fmt.Fprintf(&b, "Target difficulty: %s (derived from average proficiency %d).\n",
		diff, avgProficiency(ctx.Proficiencies))

	// Shared tags
	if len(ctx.SharedTags) > 0 {
		fmt.Fprintf(&b, "Shared interest tags: %s.\n", joinTags(ctx.SharedTags))
	} else {
		b.WriteString("Shared interest tags: (none — use each player's individual tags from the profiles below).\n")
	}

	// Profiles
	if len(ctx.Profiles) > 0 {
		b.WriteString("Player profiles:\n")
		for gcid, profile := range ctx.Profiles {
			fmt.Fprintf(&b, "  - %s: %s\n", gcid, profile)
		}
	}

	// Candidates table
	if len(ctx.Candidates) > 0 {
		b.WriteString("\nShared-atom candidates (pick by index):\n")
		for _, c := range ctx.Candidates {
			fmt.Fprintf(&b, "  [%d] Q: %s\n      Options: %s\n",
				c.Index, c.Question, joinOptions(c.Options))
		}
		b.WriteString("\nInstructions:\n")
		b.WriteString("- Choose up to Count candidate indices that best match the players' " +
			"interests (shared tags + individual tags from profiles) + target difficulty.\n")
		b.WriteString("- Generate fresh MCQs for the remainder (Count minus picks).\n")
	} else {
		b.WriteString("\nNo candidates supplied — generate ALL Count MCQs from scratch.\n")
	}
	b.WriteString("- Each generated MCQ MUST have exactly 4 options.\n")
	b.WriteString("- Each generated MCQ MUST have one unambiguous correct_answer copied " +
		"VERBATIM from the options (case-sensitive, exact match).\n")
	b.WriteString("- Calibrate generated MCQs to BOTH players' interests (shared tags + " +
		"individual tags from profiles) + target difficulty. Balance questions across " +
		"both players' topics, not just the overlap.\n")
	b.WriteString("- Use the `web_research` tool when candidates don't cover the interests " +
		"or when a question's facts need verification.\n")
	b.WriteString("- If `web_research` fails, proceed without web data — never block the duel.\n")
	b.WriteString("- No duplicate pick indices. Picks + generated MUST total Count.\n\n")

	// [EXPECTED OUTPUT]
	b.WriteString("## [EXPECTED OUTPUT]\n")
	b.WriteString("JSON only (no markdown fences, no commentary):\n")
	b.WriteString(`{"picks": [<int>...], "generated": [{"question": string, "options": [string, string, string, string], "correct_answer": string}]}` + "\n")
	b.WriteString("picks = candidate indices chosen from the pool. generated = fresh MCQs. " +
		"picks + generated MUST total Count. correct_answer MUST be one of the options " +
		"(copied verbatim).\n")

	return b.String()
}

// joinTags renders a string slice as a comma-separated list.
func joinTags(tags []string) string {
	if len(tags) == 0 {
		return "(none)"
	}
	return strings.Join(tags, ", ")
}

// joinOptions renders an options slice as a pipe-separated list.
func joinOptions(opts []string) string {
	if len(opts) == 0 {
		return "(none)"
	}
	return strings.Join(opts, " | ")
}

func safe(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
