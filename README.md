# chora-duel-atom-smith

Standalone Go ADK agent crew — the **duel_atom_smith** (P1 single-agent with
a `web_research` tool). The smith picks duel atoms from the shared-atom
candidate pool (passed in session state by chora-sharing — agents have NO DB
access, cross-DB forbidden) and generates fresh ephemeral MCQ atoms for any
shortfall, with a `web_research` tool (gateway GroundedSearch RPC, ADR-231)
when candidates/knowledge are insufficient.

Module path: `github.com/apollo-chora/chora-duel-atom-smith`.

The crew is cloud-neutral: model calls route to chora-model-gateway over gRPC
(via `chora-adk-common/modelgatewayclient`), traces go to standard OTLP (via
`chora-adk-common/tracing` → `chora-common/otel`), and secrets are env-backed.
No cloud account or managed service is required. The crew uses **no database
and no NATS** — it is a stateless HTTP agent (in-memory sessions require
replicas=1).

## Serving

The binary serves the ADK agentengine web-mode REST API on port **8080**:

```
POST /api/reasoning_engine      {"class_method":"async_create_session","input":{...}}
POST /api/stream_reasoning_engine {"class_method":"async_stream_query","input":{...}}
```

Callers create the session via `async_create_session` and pass the duel
context in session state:

```
state: {
  tenant_id:          "<tenant-uuid>",   // required by tenant propagation
  user_gcid:         "<gcid>",          // required by tenant propagation
  candidates_json:   "[...]",            // JSON array of {index, question, options}
  shared_tags_json:  "[...]",            // JSON array of strings
  proficiencies_json:"[...]",            // JSON array of ints
  profiles_json:     "{...}",            // JSON map[string]string
  count:             3,                  // target atom count
}
```

## Packages

| Package | Purpose |
|---|---|
| `cmd/duel_atom_smith/` | Entry point — wires the smith sub-agent, the web_research tool, the tenant-propagation + termination plugins, and the ADK launcher. |
| `internal/agent/` | The smith composer (pure, deterministic 6-block CREATE prompt), the ADR-197 condition extractor, and the per-turn instruction provider. |
| `internal/agentconfig/` | Build-time per-sub-agent model + prompt config (embedded YAML — the single source of truth for tier / primary_model / fallback_models / prompt_version). |
| `internal/tool/` | The `web_research` functiontool — a pure-function adapter over the model-gateway GroundedSearch RPC. |

## Configuration

| Variable | Purpose | Local default |
| --- | --- | --- |
| `CHORA_GATEWAY_ENDPOINT` | model-gateway gRPC endpoint | `gateway.chora.site:443` |
| `CHORA_GATEWAY_TOKEN` | Static bearer token for authenticated gateway calls | unset |
| `CHORA_GATEWAY_INSECURE` | Plaintext gRPC to a local gateway (dev only) | unset |
| `CHORA_GATEWAY_AUDIENCE` | Audience the gateway token was minted for (informational) | `https://gateway.chora.site` |
| `CHORA_GATEWAY_TENANT_ID` | Process fallback tenant (per-request values come from session state) | unset (required) |
| `CHORA_GATEWAY_GCID` | Process fallback gcid (per-request values come from session state) | unset (required) |
| `DUEL_ATOM_SMITH_MODEL` | Override the smith primary model (agentconfig YAML) | from YAML |
| `DUEL_ATOM_SMITH_SESSION_APP_NAME` | ADK session app name | `chora-duel-atom-smith` |
| `CHORA_ENV` | dev \| staging \| prod | `dev` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/gRPC trace endpoint | stdout |
| `CHORA_SERVICE_VERSION` | Stamped as the OTLP `service.version` attribute | `dev` |

## Build and test

```sh
go build ./...
go vet ./...
go test ./...
```

The suite is hermetic — no broker, database, gateway, or network is required.

## Docker

```sh
docker build -t chora-duel-atom-smith .
docker run --env-file .env -p 8080:8080 chora-duel-atom-smith
```
