// Command duel_atom_smith is the entry point for the duel_atom_smith crew
// (Pattern P1 single-agent with a web_research tool) per crew-composition
// SKILL.
//
// The duel_atom_smith agent picks duel atoms from the shared-atom candidate
// pool (passed in session state by chora-sharing — agents have NO DB access,
// cross-DB forbidden) and generates fresh ephemeral MCQ atoms for any
// shortfall, with a `web_research` tool (gateway GroundedSearch RPC, ADR-231)
// when candidates/knowledge are insufficient.
//
// Per ADR-138 §1 Go-first + ADR-145 polyglot agent pivot + ADR-146
// Model Broker full retirement + ADR-169 web-mode.
//
// Callers MUST create the session via `:query` endpoint with
// `class_method: async_create_session` and pass:
//
//	state: {
//	  tenant_id:          "<tenant-uuid>",   // required by tenant propagation
//	  user_gcid:         "<gcid>",          // required by tenant propagation
//	  candidates_json:   "[...]",            // JSON array of {index, question, options}
//	  shared_tags_json:  "[...]",            // JSON array of strings
//	  proficiencies_json:"[...]",            // JSON array of ints
//	  profiles_json:     "{...}",            // JSON map[string]string
//	  count:             3,                  // target atom count
//	}
//
// Env vars (NEVER inlined per feedback_no_inline_config):
//
//	DUEL_ATOM_SMITH_MODEL         — override smith primary (default from agentconfig YAML: gemini-3.5-flash)
//	CHORA_GATEWAY_ENDPOINT        — model-gateway gRPC endpoint (gateway.chora.site:443)
//	CHORA_GATEWAY_TENANT_ID       — process fallback tenant (per-request from session state)
//	CHORA_GATEWAY_GCID            — process fallback gcid (per-request from session state)
//	DUEL_ATOM_SMITH_SESSION_APP_NAME — session app name (default chora-duel-atom-smith)
//	CHORA_ENV                     — dev | staging | prod
//
// The web server port is the ADK launcher's -port flag (default 8080), set in
// the Dockerfile CMD.
package main

import (
	"context"
	"crypto/tls"
	"log"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/agentengine"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/apollo-chora/chora-adk-common/modelgatewayclient"
	"github.com/apollo-chora/chora-adk-common/promptstamping"
	"github.com/apollo-chora/chora-adk-common/terminationplugin"
	"github.com/apollo-chora/chora-adk-common/tracing"

	smithagent "github.com/apollo-chora/chora-duel-atom-smith/internal/agent"
	"github.com/apollo-chora/chora-duel-atom-smith/internal/agentconfig"
	smithtool "github.com/apollo-chora/chora-duel-atom-smith/internal/tool"

	mgv1 "github.com/apollo-chora/chora-contracts/gen/go/chora/services/model_gateway/v1"
)

const crewKind = "duel_atom_smith"

// envOr returns os.Getenv(name) if non-empty, else fallback.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	ctx := context.Background()

	// Wire the OTel exporter + W3C TraceContext propagator BEFORE any
	// agent / runner / plugin construction so every span flows to the
	// configured collector and continues an inbound traceparent. ADK Go does
	// NOT auto-wire this; per the trace wave 2026-05-29 every ADK agent
	// calls tracing.Init so it is traceable at per-agent level
	// (service.name = the registry name).
	traceShutdown, err := tracing.Init(ctx, "duel_atom_smith")
	if err != nil {
		log.Fatalf("tracing.Init: %v", err)
	}
	defer func() {
		if err := traceShutdown(context.Background()); err != nil {
			slog.Error("trace shutdown error", "err", err)
		}
	}()

	sessionAppName := envOr("DUEL_ATOM_SMITH_SESSION_APP_NAME", "chora-duel-atom-smith")

	// AGENT-DRIVEN tiering (CR mana-is-quota-not-model-selector 2026-06-01):
	// per-sub-agent model + fallback chain are config-declared in the embedded
	// agentconfig YAML (single source of truth). smith = CHEAP
	// (gemini-3.5-flash → gemini-2.5-flash). The duel path runs inside
	// processMatch's 10s ctx and MCQ selection/generation is a bounded task.
	cfg, err := agentconfig.DuelAtomSmith()
	if err != nil {
		log.Fatalf("duel_atom_smith: load agent config: %v", err)
	}
	smithCfg, err := cfg.Sub("smith")
	if err != nil {
		log.Fatalf("duel_atom_smith: %v", err)
	}
	smithModel := envOr("DUEL_ATOM_SMITH_MODEL", smithCfg.PrimaryModel)

	choraEnv := envOr("CHORA_ENV", "dev")

	slog.Info("duel_atom_smith boot",
		"session_app_name", sessionAppName,
		"smith_model", smithModel,
		"smith_tier", smithCfg.Tier,
		"smith_fallback", smithCfg.FallbackModels,
		"chora_env", choraEnv,
	)

	// Per-sub-agent model — route through chora-model-gateway (ADR-177 full
	// mana umbrella). The LLM turn flows through the one chokepoint (central
	// Model Armor, per-tenant budget, token-usage ledger). Model selection
	// stays AGENT-DRIVEN (smith=CHEAP) via the agentconfig primary +
	// forwarded fallback chain. Per-request tenant/gcid come from session
	// state via the propagation plugin; env values are the process fallback.
	gatewayEndpoint := envOr("CHORA_GATEWAY_ENDPOINT", "gateway.chora.site:443")
	// D6 step 1: the token audience is read HERE and defaulted explicitly.
	// modelgatewayclient still defaults it internally in TWO places
	// (client.go:181-182 and image.go:110-111); passing it makes the value
	// stateable and is what lets step 4 remove those defaults safely.
	gatewayAudience := envOr("CHORA_GATEWAY_AUDIENCE", "https://gateway.chora.site")
	gatewayTenantID := os.Getenv("CHORA_GATEWAY_TENANT_ID")
	gatewayGCID := os.Getenv("CHORA_GATEWAY_GCID")
	if gatewayTenantID == "" || gatewayGCID == "" {
		log.Fatalf("duel_atom_smith: CHORA_GATEWAY_TENANT_ID + CHORA_GATEWAY_GCID required " +
			"(process fallback; per-request values come from session state — no silent " +
			"mis-attribution per ADR-169 + feedback_no_stubs_real_wiring)")
	}
	smithGemini, err := modelgatewayclient.New(ctx, smithGatewayConfig(crewKind, gatewayEndpoint, gatewayGCID, gatewayTenantID, smithCfg, smithModel, gatewayAudience))
	if err != nil {
		log.Fatalf("duel_atom_smith: modelgatewayclient.New(smith, %s): %v", smithModel, err)
	}

	// gRPC client for the web_research tool's GroundedSearch RPC (ADR-231).
	// This duplicates the TLS + bearer-token dial from
	// modelgatewayclient/client.go:204-233 — no shared helper exists for
	// constructing a raw mgv1.ModelGatewayServiceClient (modelgatewayclient.New
	// returns an adkmodel.LLM, not the raw gRPC client). The web_research
	// tool needs the raw client to call GroundedSearch directly.
	// otelgrpc client handler injects the active span's W3C trace context
	// into outgoing gRPC metadata so the gateway's span parents under this
	// agent's span.
	mgDialOpts := []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	if os.Getenv("CHORA_GATEWAY_INSECURE") == "1" {
		// LOCAL DEV ONLY — plaintext gRPC to a local gateway.
		mgDialOpts = append(mgDialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// D6 step 3. This used to be its OWN hardcoded copy of the audience
		// literal, minted outside modelgatewayclient entirely, so a perfect fix
		// inside that library would have left this agent on a second source of
		// truth. It now reads the same value the client config carries.
		// Cloud-neutral auth: the same static bearer token modelgatewayclient
		// dials with (CHORA_GATEWAY_TOKEN). The local model gateway does not
		// enforce token auth; deployments fronting it with an authenticating
		// proxy read the token from the environment.
		token := strings.TrimSpace(os.Getenv(modelgatewayclient.EnvGatewayToken))
		if token == "" {
			log.Fatalf("duel_atom_smith: %s is required for authenticated gateway calls "+
				"(or CHORA_GATEWAY_INSECURE=1 for local dev)", modelgatewayclient.EnvGatewayToken)
		}
		mgDialOpts = append(mgDialOpts,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})),
			grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: staticTokenSource{token: token}}),
		)
	}
	mgConn, err := grpc.NewClient(gatewayEndpoint, mgDialOpts...)
	if err != nil {
		log.Fatalf("duel_atom_smith: grpc.NewClient(%s): %v", gatewayEndpoint, err)
	}
	defer mgConn.Close()
	mgClient := mgv1.NewModelGatewayServiceClient(mgConn)

	// web_research tool — grounded web search via the gateway's GroundedSearch
	// RPC (ADR-231). Used when candidates don't cover the interests or facts
	// need verification. The agent's instruction tells it to proceed without
	// web data on tool failure.
	webResearchTool, err := smithtool.NewWebResearchTool(mgClient)
	if err != nil {
		log.Fatalf("duel_atom_smith: NewWebResearchTool: %v", err)
	}

	// Per-request tenant propagation (ADR-169) — stamp tenant_id/user_gcid from
	// session state onto every gateway Invoke. No ActionCodeResolver: the
	// action code is static (duel_atom_smith).
	tenantPropP, err := modelgatewayclient.NewTenantPropagationPlugin(crewKind)
	if err != nil {
		log.Fatalf("duel_atom_smith: modelgatewayclient.NewTenantPropagationPlugin: %v", err)
	}

	// P1 single-agent — one llmagent.New with the web_research tool, then
	// adkagent.NewSingleLoader. NO sequentialagent, NO loopagent.
	// Per-turn instruction composition (ADR-169 migration). The candidates
	// / tags / proficiencies / profiles / count are unknown at boot, so the
	// smith sub-agent recomposes its instruction from the session state the
	// chora-sharing caller populates at async_create_session.
	// InstructionProvider takes precedence over the static Instruction field.
	smithAgent, err := llmagent.New(llmagent.Config{
		Name:        "smith",
		Model:       smithGemini,
		Description: "Picks duel atoms from the shared-atom candidate pool + generates fresh MCQ atoms for any shortfall (CHEAP tier; bounded MCQ task inside the duel path's 10s ctx).",
		InstructionProvider: promptstamping.WithStamping(
			smithCfg.PromptVersion,
			smithagent.SmithConditions(smithagent.RoleSmith),
			smithagent.NewInstructionProvider(smithagent.RoleSmith),
		),
		Tools: []tool.Tool{webResearchTool},
	})
	if err != nil {
		log.Fatalf("duel_atom_smith: llmagent.New(smith): %v", err)
	}
	loader := adkagent.NewSingleLoader(smithAgent)

	// NOTE: the tieredmodelplugin (ADR-149 mana × growth LLM matrix) is
	// DELIBERATELY NOT registered here (CR mana-is-quota-not-model-selector
	// 2026-06-01, user directive — mirrors qgen/moderation/familiar). Mana is
	// a token-budget QUOTA system — it must NOT dictate which LLM model is
	// used. Model selection is AGENT-DRIVEN via the per-sub-agent agentconfig
	// YAML (smith=cheap gemini-3.5-flash).

	// MaxIterations = 5 — allows ≤2 web-research rounds + final answer.
	// The smith is a P1 single-agent: pick/generate → (optional web_research)
	// → final answer. 5 iterations covers the warmest path with retries.
	terminationP, err := terminationplugin.New(terminationplugin.Config{
		Publisher:     &terminationplugin.LoggingPublisher{},
		AgentID:       "duel_atom_smith",
		Runtime:       "AGENT_EXECUTION_RUNTIME_ADK_GO",
		CrewKind:      crewKind,
		CrewPattern:   "P1_SINGLE",
		MaxIterations: 5,
	})
	if err != nil {
		log.Fatalf("duel_atom_smith: terminationplugin.New: %v", err)
	}

	// SessionService — in-memory (ADR-169 migration 2026-06-01). The crew
	// runs as a plain Service (the ADK agentengine launcher's `web` mode
	// serves HTTP). An in-memory session store removes the managed-agent
	// platform's session dependency. NOTE: in-memory sessions require
	// replicas=1.
	sessionService := session.InMemoryService()

	config := &launcher.Config{
		SessionService: sessionService,
		AgentLoader:    loader,
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{tenantPropP, terminationP},
		},
	}

	// NewLauncher's arg becomes the ADK session AppName (NOT an upstream RPC
	// target in web mode — every method handler is constructed with this value,
	// so create/stream/delete agree on the session namespace). Pass the decoupled
	// sessionAppName so every session.Create + StreamQuery carries a non-empty
	// app_name. Mirrors chora-familiar / chora-moderation.
	l := agentengine.NewLauncher(sessionAppName)
	if err := l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

// crewSurface is the ADR-254 D7 surface value of this crew.
const crewSurface = "duel_atom_smith"

// staticTokenSource is an oauth2.TokenSource over a fixed bearer token —
// the cloud-neutral gateway credential (mirrors modelgatewayclient's
// internal staticTokenSource).
type staticTokenSource struct{ token string }

func (s staticTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: s.token, TokenType: "Bearer"}, nil
}

// smithGatewayConfig is the gateway client identity of the duel_atom_smith call. Surface is
// the crew id (ADR-254 D7): the D7 gateway refuses an unstamped Invoke
// (FAILED_PRECONDITION surface_unstamped), so it is set here, once, and
// asserted by a test; everything else is what the call always sent.
func smithGatewayConfig(crewKind string, gatewayEndpoint string, gatewayGCID string, gatewayTenantID string, smithCfg agentconfig.SubAgentConfig, smithModel string, gatewayAudience string) modelgatewayclient.Config {
	return modelgatewayclient.Config{
		Endpoint:         gatewayEndpoint,
		LogicalModelID:   smithModel,
		FallbackModelIDs: smithCfg.FallbackModels,
		AgentID:          "duel_atom_smith",
		CrewKind:         crewKind,
		TenantID:         gatewayTenantID,
		GCID:             gatewayGCID,
		Audience:         gatewayAudience,
		ActionCode:       "duel_atom_smith",
		Insecure:         os.Getenv("CHORA_GATEWAY_INSECURE") == "1",
		Surface:          crewSurface,
	}
}
