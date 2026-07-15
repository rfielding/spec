# Proto Conversation Spec

This repo sketches a Go compiler for actor-local protocol specifications.

- `.proto` files define serialized messages.
- `.convspec` files define observable actor behavior in Lisp syntax.
- the compiled model renders state machines, interaction scenarios, metrics, and CTL checks.

The current language is intentionally actor-local. A state belongs to an `(actor ...)` block, and `(on Message ...)` means that actor received `Message` from its bounded FIFO inbox. Message origin is not part of the handler syntax; if a return address or source identity matters, put it in the protobuf message.

## Example

```text
(spec auth
  (import "auth.proto")
  (participants server)

  (conversation login
    (start Idle)

    (actor server
      (state Idle
        (on LoginRequest
          (when (and (!= msg.username "") (!= msg.password "")))
          (then Authenticated (chance 0.90))
          (then Rejected (chance otherwise))))

      (state Authenticated accept
        (state_is authenticated)
        (state_is terminal))

      (state Rejected accept
        (state_is rejected)
        (state_is terminal)))))
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
