package main

// Interim W3 stamp (ADR-254 D7, coordinator ruling 2026-08-22 17:57Z): every
// gateway client of this web-mode binary names its crew as the Invoke
// surface; the D7 gateway refuses an unstamped call. The tests pin the stamp
// per client so a future edit cannot drop it silently.
//
// The model id is env-only: the tests set DUEL_ATOM_SMITH_MODEL and assert the
// env-provided value is the LogicalModelID stamped on the gateway client, and
// that the optional fallback list is comma-split from its own env var.

import (
	"strings"
	"testing"
)

func Test_smithGatewayConfig_stampsTheSurface(t *testing.T) {
	// "fb" is a synthetic fallback sentinel — it stands in for a second model so
	// the assertion still tells LogicalModelID and FallbackModelIDs apart; the
	// deployment itself serves a single text route.
	const wantModel = "env-smith-model"
	t.Setenv(envSmithModel, wantModel)
	model, err := requiredModel(envSmithModel)
	if err != nil {
		t.Fatalf("requiredModel: %v", err)
	}
	cfg := smithGatewayConfig("crew-x", "gw:443", "gcid-1", "tenant-1", model, []string{"fb"}, "https://gateway.test.invalid")
	if cfg.Surface != crewSurface || crewSurface != "duel_atom_smith" {
		t.Fatalf("surface = %q, want the ADR-254 D7 crew id duel_atom_smith", cfg.Surface)
	}
	if cfg.AgentID != "duel_atom_smith" || cfg.Endpoint != "gw:443" || cfg.TenantID != "tenant-1" || cfg.GCID != "gcid-1" || cfg.CrewKind != "crew-x" || cfg.LogicalModelID != wantModel || len(cfg.FallbackModelIDs) != 1 || cfg.FallbackModelIDs[0] != "fb" {
		t.Fatalf("identity changed: %+v", cfg)
	}
	if cfg.ActionCode != "duel_atom_smith" {
		t.Fatalf("action code = %q, want %q", cfg.ActionCode, "duel_atom_smith")
	}
}

func Test_requiredModel_failsLoudWhenUnset(t *testing.T) {
	t.Setenv(envSmithModel, "")
	if _, err := requiredModel(envSmithModel); err == nil || !strings.Contains(err.Error(), envSmithModel) {
		t.Fatalf("an unset model var must fail loud and name the var: %v", err)
	}
	// A whitespace-only value is unset, not a model id.
	t.Setenv(envSmithModel, "  ")
	if _, err := requiredModel(envSmithModel); err == nil {
		t.Fatal("a blank model var must fail loud")
	}
}

func Test_fallbackModels_envDrivenCommaList(t *testing.T) {
	// Unset (the default) declares no chain: the gateway is asked for the
	// primary target only.
	t.Setenv(envSmithFallback, "")
	if got := fallbackModels(envSmithFallback); len(got) != 0 {
		t.Fatalf("unset fallback list = %v, want none", got)
	}
	t.Setenv(envSmithFallback, " env-fb-a , env-fb-b ,, ")
	got := fallbackModels(envSmithFallback)
	if len(got) != 2 || got[0] != "env-fb-a" || got[1] != "env-fb-b" {
		t.Fatalf("fallback list = %v, want [env-fb-a env-fb-b]", got)
	}
}
