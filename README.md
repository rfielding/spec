# Proto Conversation Spec

This repo sketches a Go compiler for actor-local protocol specifications.

- `.proto` files define serialized messages.
- `.convspec` files define observable actor behavior in Lisp syntax.
- the compiled model renders state machines, interaction scenarios, metrics, and CTL checks.

The current language is intentionally actor-local. A state belongs to an `(actor ...)` block, and `(on Message ...)` means that actor received `Message` from its single spec-wide bounded FIFO inbox. Message origin is not part of the handler syntax; if a return address or source identity matters, put it in the protobuf message.

Each conversation starts when an actor consumes a protobuf activation message from that inbox. Actor state machines are then defined from scratch for that conversation; actor-wide resources stay at spec scope.

The project now includes a self-model at [examples/spec_model.convspec](examples/spec_model.convspec) with protobuf messages in [examples/spec_model.proto](examples/spec_model.proto). It is the Swagger-like target for this tool: completely realized message serialization, actor capacity for queueing metrics, probabilities for MDP-style metrics, declared line/pie chart views, and CTL assertions over observable states. See [docs/spec-model.md](docs/spec-model.md).

## Example

```text
(spec auth
  (import "auth.proto")
  (include "auth_login.convspec")
  (actor server (capacity 64))
)
```

`examples/auth_login.convspec`:

```text
(conversation login
  (start server LoginConversationStarted Idle)
  ...)
```

## Go Compiler

```bash
go run ./cmd/convspec examples/auth.convspec
go run ./cmd/convspec examples/auth.convspec --format html -o build/auth.html
go run ./cmd/convspec examples/auth.convspec --format dot
go run ./cmd/convspec examples/auth.convspec --format mermaid-sequence
go run ./cmd/convspec examples/auth.convspec --format checks
go run ./cmd/convspec examples/auth.convspec --format metrics
go run ./cmd/convspec examples/auth.convspec --format json -o build/auth.json
go run ./cmd/convspec examples/spec_model.convspec --format html -o build/spec_model.html
```

Formats:

- `html`: browser page with Graphviz state machine and SVG interaction scenarios.
- `mermaid`: one state diagram per conversation.
- `mermaid-sequence`: one sequence diagram per acyclic terminal path.
- `dot`: Graphviz DOT state graph.
- `checks`: CTL assertion results.
- `metrics`: estimated outcome, dwell-time, byte, inbox, and reliability metrics.
- `json`: compiler model for later tooling.

Run the chat workbench locally:

```bash
go run ./cmd/specweb
```

Run tests:

```bash
go test ./...
```

See [docs/conversation-spec.md](docs/conversation-spec.md) for the language model and [docs/evidence-workbench.md](docs/evidence-workbench.md) for the workbench direction.
