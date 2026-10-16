package tool_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/apollo-chora/chora-duel-atom-smith/internal/tool"
	gatewayv1 "github.com/apollo-chora/chora-duel-atom-smith/internal/tool/gatewayv1"
	"google.golang.org/grpc"
)

// fakeMGClient is a minimal ModelGatewayServiceClient for testing the
// web_research tool without a live gateway. It implements Invoke,
// GroundedSearch and Embed (the interface requires all three).
type fakeMGClient struct {
	groundedResp *gatewayv1.GroundedSearchResponse
	groundedErr  error
	lastReq      *gatewayv1.GroundedSearchRequest
}

// InvokeStream and ListModels are not exercised by these tests; the
// ModelGatewayServiceClient interface requires them.
func (f *fakeMGClient) InvokeStream(ctx context.Context, in *gatewayv1.InvokeRequest, opts ...grpc.CallOption) (gatewayv1.ModelGatewayService_InvokeStreamClient, error) {
	return nil, fmt.Errorf("fakeMGClient: InvokeStream not implemented")
}

func (f *fakeMGClient) ListModels(ctx context.Context, in *gatewayv1.ListModelsRequest, opts ...grpc.CallOption) (*gatewayv1.ListModelsResponse, error) {
	return nil, fmt.Errorf("fakeMGClient: ListModels not implemented")
}

func (f *fakeMGClient) Invoke(ctx context.Context, in *gatewayv1.InvokeRequest, opts ...grpc.CallOption) (*gatewayv1.InvokeResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMGClient) Embed(ctx context.Context, in *gatewayv1.EmbedRequest, opts ...grpc.CallOption) (*gatewayv1.EmbedResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMGClient) GroundedSearch(ctx context.Context, in *gatewayv1.GroundedSearchRequest, opts ...grpc.CallOption) (*gatewayv1.GroundedSearchResponse, error) {
	f.lastReq = in
	if f.groundedErr != nil {
		return nil, f.groundedErr
	}
	if f.groundedResp != nil {
		return f.groundedResp, nil
	}
	return &gatewayv1.GroundedSearchResponse{}, nil
}

func TestWebResearch_HappyPath(t *testing.T) {
	client := &fakeMGClient{
		groundedResp: &gatewayv1.GroundedSearchResponse{
			GroundedAnswer: "Paris is the capital of France.",
			Citations: []*gatewayv1.GroundedCitation{
				{Uri: "https://example.com/paris", Title: "Paris - Wikipedia"},
				{Uri: "https://example.com/france", Title: "France Geography"},
			},
			WebSearchQueries: []string{"capital of France"},
		},
	}

	out, err := tool.RunGroundedSearch(context.Background(), client, "tenant-1", "gcid-1", "traceparent-1", "capital of France")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Answer != "Paris is the capital of France." {
		t.Errorf("Answer = %q; want the grounded answer", out.Answer)
	}
	if len(out.Citations) != 2 {
		t.Fatalf("Citations len = %d; want 2", len(out.Citations))
	}
	if out.Citations[0].URI != "https://example.com/paris" {
		t.Errorf("Citations[0].URI = %q", out.Citations[0].URI)
	}
	if out.Citations[0].Title != "Paris - Wikipedia" {
		t.Errorf("Citations[0].Title = %q", out.Citations[0].Title)
	}
	if len(out.Queries) != 1 || out.Queries[0] != "capital of France" {
		t.Errorf("Queries = %v; want [capital of France]", out.Queries)
	}

	// Verify the RPC request fields.
	if client.lastReq.AgentId != "duel_atom_smith" {
		t.Errorf("AgentId = %q; want duel_atom_smith", client.lastReq.AgentId)
	}
	if client.lastReq.CrewKind != "duel_atom_smith" {
		t.Errorf("CrewKind = %q; want duel_atom_smith", client.lastReq.CrewKind)
	}
	if client.lastReq.ActionCode != "duel_atom_smith_web_research" {
		t.Errorf("ActionCode = %q; want duel_atom_smith_web_research", client.lastReq.ActionCode)
	}
	// The gateway resolves the grounded-search operation from the RPC method,
	// so the caller sends no model id. The old contract carried a hardcoded
	// "gemini-2.5-flash" here.
	if client.lastReq.MaxResults != 5 {
		t.Errorf("MaxResults = %d; want 5", client.lastReq.MaxResults)
	}
	if client.lastReq.TenantId != "tenant-1" {
		t.Errorf("TenantId = %q; want tenant-1", client.lastReq.TenantId)
	}
	if client.lastReq.Gcid != "gcid-1" {
		t.Errorf("Gcid = %q; want gcid-1", client.lastReq.Gcid)
	}
	if client.lastReq.Traceparent != "traceparent-1" {
		t.Errorf("Traceparent = %q; want traceparent-1", client.lastReq.Traceparent)
	}
	if client.lastReq.InvocationId == "" {
		t.Error("InvocationId must be non-empty (UUID)")
	}
}

func TestWebResearch_RPCError(t *testing.T) {
	client := &fakeMGClient{
		groundedErr: errors.New("rpc error: code = Unavailable"),
	}

	_, err := tool.RunGroundedSearch(context.Background(), client, "t", "g", "", "test query")
	if err == nil {
		t.Fatal("expected error on RPC failure; got nil")
	}
	if !strings.Contains(err.Error(), "GroundedSearch RPC") {
		t.Errorf("error must wrap the RPC failure; got %q", err.Error())
	}
}

func TestWebResearch_TruncationAt120Runes(t *testing.T) {
	client := &fakeMGClient{
		groundedResp: &gatewayv1.GroundedSearchResponse{},
	}

	// Build a query of 200 runes (each char is 1 rune).
	longQuery := strings.Repeat("x", 200)

	_, err := tool.RunGroundedSearch(context.Background(), client, "t", "g", "", longQuery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	directive := client.lastReq.Directive
	if got := len([]rune(directive)); got != 120 {
		t.Errorf("directive rune count = %d; want 120 (truncated)", got)
	}
}

func TestWebResearch_EmptyQuery(t *testing.T) {
	client := &fakeMGClient{}

	_, err := tool.RunGroundedSearch(context.Background(), client, "t", "g", "", "   ")
	if err == nil {
		t.Fatal("expected error on empty query; got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error must mention empty query; got %q", err.Error())
	}
}

func TestWebResearch_NilClient(t *testing.T) {
	_, err := tool.RunGroundedSearch(context.Background(), nil, "t", "g", "", "test")
	if err == nil {
		t.Fatal("expected error on nil client; got nil")
	}
}
