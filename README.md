# chora-duel-atom-smith

## About

`chora-duel-atom-smith` is a standalone Go service built with Google ADK. Its `smith` agent assembles duel rounds by selecting suitable atoms from a candidate pool and generating fresh multiple-choice questions when more atoms are needed. The agent can use the `web_research` tool, backed by the Chora model gateway's GroundedSearch RPC, when the supplied candidates are insufficient or facts need verification.

The service does not access a database or NATS. It keeps ADK sessions in memory and routes model calls to `chora-model-gateway` over gRPC.

## Quick start

Prerequisites:

- Go 1.26.6 or newer
- Access to a Chora model gateway
- A gateway tenant ID and user GCID
- A gateway bearer token unless using the local plaintext development mode

For local development with a gateway running on the host, copy the example environment file and fill in the required tenant and GCID values:

```sh
cp .env.example .env
go run ./cmd/duel_atom_smith web -port 8080 agentengine
```

The example configuration uses `host.docker.internal:9090` and `CHORA_GATEWAY_INSECURE=1` for a local gateway. For a deployed gateway, set `CHORA_GATEWAY_ENDPOINT`, `CHORA_GATEWAY_TOKEN`, and the tenant identity variables appropriately.

To build the binary:

```sh
go build -o duel_atom_smith ./cmd/duel_atom_smith
./duel_atom_smith web -port 8080 agentengine
```

## Usage

The service exposes the ADK Agent Engine web-mode API on port 8080 by default. The container image starts the same web launcher with:

```text
web -port 8080 agentengine
```

Create a session with `async_create_session`, then stream a query with `async_stream_query`:

```http
POST /api/reasoning_engine
Content-Type: application/json

{"class_method":"async_create_session","input":{...}}
```

```http
POST /api/stream_reasoning_engine
Content-Type: application/json

{"class_method":"async_stream_query","input":{...}}
```

The caller supplies duel context in the session state. The smith reads these keys on each turn:

```text
tenant_id           string
user_gcid           string
candidates_json     JSON array of {index, question, options}
shared_tags_json    JSON array of strings
proficiencies_json  JSON array of integers
profiles_json       JSON object/map of player profile strings
count               target atom count
```

The smith returns JSON in this shape:

```json
{
  "picks": [0, 2],
  "generated": [
    {
      "question": "Example question",
      "options": ["A", "B", "C", "D"],
      "correct_answer": "B"
    }
  ]
}
```

`picks` contains candidate indexes. `generated` contains new MCQs. The prompt requires the combined count of picks and generated atoms to equal the requested `count`, generated questions to have exactly four options, and `correct_answer` to match one option exactly.

Configuration is environment-based:

| Variable | Description | Default |
| --- | --- | --- |
| `CHORA_GATEWAY_ENDPOINT` | Model gateway gRPC endpoint | `gateway.chora.site:443` |
| `CHORA_GATEWAY_TOKEN` | Bearer token used for authenticated gateway calls | unset |
| `CHORA_GATEWAY_INSECURE` | Use plaintext gRPC for local development | unset |
| `CHORA_GATEWAY_AUDIENCE` | Gateway token audience | `https://gateway.chora.site` |
| `CHORA_GATEWAY_TENANT_ID` | Process-level fallback tenant ID | unset |
| `CHORA_GATEWAY_GCID` | Process-level fallback user GCID | unset |
| `DUEL_ATOM_SMITH_MODEL` | Override the configured smith primary model | configured value |
| `DUEL_ATOM_SMITH_SESSION_APP_NAME` | ADK session application name | `chora-duel-atom-smith` |
| `CHORA_ENV` | Environment name | `dev` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/gRPC trace endpoint | stdout when unset |
| `CHORA_SERVICE_VERSION` | OTLP `service.version` value | `dev` |

The smith model configuration is embedded in `internal/agentconfig/duel_atom_smith.yaml`. It currently declares the `cheap` tier with `gemini-3.5-flash` as the primary model and `gemini-2.5-flash` as the fallback.

To run the container image:

```sh
docker build -t chora-duel-atom-smith .
docker run --env-file .env -p 8080:8080 chora-duel-atom-smith
```

The service uses in-memory sessions, so deployment with multiple replicas is not supported without an external session store.

## Development

The repository is a Go module:

```text
github.com/apollo-chora/chora-duel-atom-smith
```

The main packages are:

| Path | Purpose |
| --- | --- |
| `cmd/duel_atom_smith/` | Service entry point and ADK launcher wiring |
| `internal/agent/` | Prompt composition, session-state decoding, and per-turn instruction provider |
| `internal/agentconfig/` | Embedded smith model and prompt configuration |
| `internal/tool/` | `web_research` adapter for the model gateway GroundedSearch RPC |

Run the repository checks with:

```sh
gofmt -w .
go mod tidy
go vet ./...
go test ./...
```

CI runs formatting checks, verifies that `go mod tidy` produces no module changes, then runs `go vet ./...` and `go test ./...`.

The test suite covers prompt composition, session-state handling, agent configuration, gateway configuration, and the `web_research` tool. It does not require a running broker, database, or gateway.
