# Proto Conversation Spec

This repo sketches a specification system built around a strict boundary:

- `.proto` files define atomic serialized messages.
- `.convspec` files define valid conversations built from those messages.
- the compiled observable protocol becomes a state machine / Kripke structure for temporal checking

The key idea is that protobuf should not try to encode protocol flow. A conversation spec imports message types from protobuf, then defines:

- participants
- states
- allowed message directions
- guards over message fields
- correlation rules between messages
- terminal and error paths
- observable propositions for model checking

See [docs/conversation-spec.md](/home/rfielding/code/spec/docs/conversation-spec.md) for the language and model, [examples/auth.proto](/home/rfielding/code/spec/examples/auth.proto) with [examples/auth.convspec](/home/rfielding/code/spec/examples/auth.convspec) for a minimal example, and [examples/reservation.proto](/home/rfielding/code/spec/examples/reservation.proto) with [examples/reservation.convspec](/home/rfielding/code/spec/examples/reservation.convspec) for a versioned reservation protocol that is intended to compile into a CTL-checkable state machine.

See [docs/evidence-workbench.md](/home/rfielding/code/spec/docs/evidence-workbench.md) for the intended direction: a web-based design workbench where chat responses can include deterministic diagrams, temporal-check results, counterexample traces, and metrics views.

## Go Compiler

The repository includes a dependency-free Go compiler that reads a `.convspec` file, indexes its imported `.proto` messages, validates references, and emits diagrams or JSON.

```bash
go run ./cmd/convspec examples/auth.convspec
go run ./cmd/convspec examples/reservation.convspec --format html -o build/reservation.html
go run ./cmd/convspec examples/reservation.convspec --format dot
go run ./cmd/convspec examples/reservation.convspec --format mermaid-sequence
go run ./cmd/convspec examples/reservation.convspec --format json -o build/reservation.json
```

Formats:

- `html`: browser page with Graphviz-rendered PNG state and path diagrams.
- `mermaid`: one state diagram per conversation, showing every legal branch.
- `mermaid-sequence`: one sequence diagram per acyclic terminal path.
- `dot`: Graphviz DOT state graph.
- `json`: compiler model for later tooling.

Open the generated HTML file directly in a browser:

```bash
go run ./cmd/convspec examples/reservation.convspec --format html -o build/reservation.html
```

The HTML generator invokes `dot -Tpng`, writes image files next to the report, and links them from the page. If a diagram cannot compile, generation fails instead of producing a broken browser page.

Current compiler scope:

- Supports the DSL used in `examples/*.convspec`.
- Validates participants, message type names, start states, and transition targets.
- Parses enough proto syntax to discover top-level message names and fields.
- Does not yet use `protoc` descriptors or evaluate guard expressions semantically.

The compiler library lives under `internal/convspec`, with the command-line wrapper in `cmd/convspec`. That split is deliberate: the next tool can be a Go web server that imports the compiler, serves the rendered diagrams at a URL, and lets a user discuss proposed spec changes with an LLM against the same compiled model.

Run tests with:

```bash
go test ./...
```
