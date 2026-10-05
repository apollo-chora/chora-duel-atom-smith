// Package tool holds the stateless tool functions called by the duel_atom_smith
// ADK agent. Each is a pure-function adapter over an upstream Chora gRPC
// service. NO domain logic lives here — only the adapter wiring.
//
// Per `adk-tool-calling-loop` SKILL conventions: tools emit OTLP spans
// with OpenInference semantic conventions.
package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	mgv1 "github.com/apollo-chora/chora-contracts/gen/go/chora/services/model_gateway/v1"
)

// maxDirectiveRunes is the proto directive cap (GroundedSearchRequest.directive
// is ≤120 chars per the proto comment). Queries exceeding this are truncated.
const maxDirectiveRunes = 120

// WebResearchInput is the agent-provided input to the web_research tool.
type WebResearchInput struct {
	Query string `json:"query"`
}

// WebResearchCitation is one grounded citation.
type WebResearchCitation struct {
	URI   string `json:"uri"`
	Title string `json:"title"`
}

// WebResearchOutput is the tool's output.
type WebResearchOutput struct {
	Answer    string                `json:"answer"`
	Citations []WebResearchCitation `json:"citations"`
	Queries   []string              `json:"queries"`
}

// NewWebResearchTool constructs the web_research functiontool backed by the
// model-gateway GroundedSearch RPC (ADR-231). The mgClient is the gRPC client
// constructed in main.go with the same TLS + bearer-token dial as
// modelgatewayclient (no shared helper exists, so main.go duplicates the dial).
func NewWebResearchTool(mgClient mgv1.ModelGatewayServiceClient) (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "web_research",
			Description: "Grounded web search via the platform's single controlled egress (model-gateway GroundedSearch, ADR-231). Use when the shared-atom candidates do not cover the players' interests, or when a question's facts need verification.",
		},
		func(tctx tool.Context, in WebResearchInput) (WebResearchOutput, error) {
			// Resolve tenant_id + user_gcid + traceparent from session state.
			// The tool.Context embeds agent.CallbackContext → ReadonlyContext →
			// ReadonlyState(). Missing keys yield empty strings (safe fallback).
			var tenantID, gcid, traceparent string
			if tctx != nil {
				if st := tctx.ReadonlyState(); st != nil {
					tenantID = stateStringFromTool(st, "tenant_id")
					gcid = stateStringFromTool(st, "user_gcid")
					traceparent = stateStringFromTool(st, "traceparent")
				}
			}
			ctx := context.Background()
			if tctx != nil {
				ctx = tctx
			}
			return RunGroundedSearch(ctx, mgClient, tenantID, gcid, traceparent, in.Query)
		},
	)
}

// stateStringFromTool reads a string from a session.ReadonlyState.
func stateStringFromTool(st interface{ Get(string) (any, error) }, key string) string {
	v, err := st.Get(key)
	if err != nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// RunGroundedSearch executes the GroundedSearch RPC with explicit context +
// identity fields. This is the testable core — separated from the
// tool.Context adapter so unit tests can call it directly without
// constructing a full ADK tool.Context. RPC errors are returned as tool
// errors — the agent's instruction tells it to proceed without web data on
// tool failure.
func RunGroundedSearch(ctx context.Context, mgClient mgv1.ModelGatewayServiceClient, tenantID, gcid, traceparent, query string) (WebResearchOutput, error) {
	if mgClient == nil {
		return WebResearchOutput{}, errors.New("web_research: model-gateway client is nil")
	}

	q := strings.TrimSpace(query)
	if q == "" {
		return WebResearchOutput{}, errors.New("web_research: query is empty")
	}

	// Truncate to the proto directive cap (≤120 runes).
	if r := []rune(q); len(r) > maxDirectiveRunes {
		q = string(r[:maxDirectiveRunes])
	}

	resp, err := mgClient.GroundedSearch(ctx, &mgv1.GroundedSearchRequest{
		InvocationId:   uuid.Must(uuid.NewV7()).String(),
		TenantId:       tenantID,
		Gcid:           gcid,
		AgentId:        "duel_atom_smith",
		CrewKind:       "duel_atom_smith",
		Directive:      q,
		MaxResults:     5,
		LogicalModelId: "gemini-2.5-flash",
		ActionCode:     "duel_atom_smith_web_research",
		Traceparent:    traceparent,
	})
	if err != nil {
		return WebResearchOutput{}, fmt.Errorf("web_research: GroundedSearch RPC: %w", err)
	}

	out := WebResearchOutput{
		Answer:  resp.GetGroundedAnswer(),
		Queries: resp.GetWebSearchQueries(),
	}
	for _, c := range resp.GetCitations() {
		out.Citations = append(out.Citations, WebResearchCitation{
			URI:   c.GetUri(),
			Title: c.GetTitle(),
		})
	}
	return out, nil
}
