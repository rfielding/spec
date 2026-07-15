# Spec Model

`examples/spec_model.convspec` is a first model of this project in its own language. It is intentionally similar in spirit to Swagger: protobuf defines the serialized messages, and convspec defines the observable protocol, probabilities, queues, assertions, and documentation views around those messages.

The current model establishes these commitments:

- every consumed message is a protobuf message, so nominal byte size can be estimated from the schema
- every actor has a bounded FIFO inbox, and writes are modeled as blocking when the inbox is full
- chance annotations represent black-box implementation choices or stochastic user/tool behavior
- declared metric views name the charts expected from causal traffic logs
- CTL assertions describe reachability and eventual completion over observable protocol states

Run it with:

```bash
go run ./cmd/convspec examples/spec_model.convspec --format html -o build/spec_model.html
go run ./cmd/convspec examples/spec_model.convspec --format metrics
go run ./cmd/convspec examples/spec_model.convspec --format json
```

The compiler does not yet derive queue arrival rates from upstream message writes or run the full MDP. The spec is written so those pieces have obvious homes: message traffic produces byte events, chance nodes produce transition probabilities, inbox declarations provide capacities, and `(metric ...)` declarations identify the line and pie charts to compute from causal traffic.
