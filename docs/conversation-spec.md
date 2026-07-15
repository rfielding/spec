# Conversation Spec

Conversation specs describe externally observable actor behavior over protobuf messages.

Protobuf owns the wire message schema. Convspec owns the legal message-driven behavior, temporal assertions, and deterministic documentation views.

## Actor-Local Model

A conversation compiles to a labeled transition system:

- states are protocol states owned by an actor
- transitions are messages consumed from that actor's FIFO inbox
- guards refer to fields on the current message as `msg`
- terminal states are marked `accept` or `reject`
- `state_is` labels become propositions for CTL

The spec does not write `from` or `to` on handlers. Inside `(actor server ...)`, every `(on Message ...)` handles a `Message` received by `server`. If a source actor, return address, or actor instance matters, it belongs in the protobuf message.

## Core Syntax

```text
(spec auth
  (import "auth.proto")
  (participants server)

  (conversation login
    (start Idle)

    (assert eventually_done
      (always (mustEventually (or Authenticated Rejected))))

    (actor server
      (state Idle
        (on LoginRequest
          (when (and (!= msg.username "") (!= msg.password "")) then Authenticated (chance 0.90))
          (when (and (!= msg.username "") (!= msg.password "")) then Rejected (chance otherwise))))

      (state Authenticated accept
        (state_is authenticated)
        (state_is terminal))

      (state Rejected accept
        (state_is rejected)
        (state_is terminal)))))
```

### `spec`

```text
(spec <name> ...)
```

Names the module.

### `import`

```text
(import "auth.proto")
```

Loads protobuf messages used by `on` handlers.

### `participants`

```text
(participants server)
```

Declares logical actor roles. Actor instances, such as `truck-1` or `storefront-4`, should be modeled through message fields or later instance-binding syntax rather than hard-coded into every handler.

### `conversation`

```text
(conversation login
  (start Idle)
  ...)
```

Defines one protocol graph. A version can be attached with `(version 2)`.

### `actor`

```text
(actor server
  (state Idle ...))
```

Groups states owned by one actor. Handlers inside those states consume from that actor's inbox.

### `state`

```text
(state Authenticated accept
  (state_is authenticated)
  (state_is terminal))
```

Defines a protocol state. `accept` and `reject` are terminal markers. `state_is` labels the current state with a proposition used by CTL.

### `on`

```text
(on LoginRequest
  (when (and (!= msg.username "") (!= msg.password "")) then Authenticated (chance 0.90))
  (when (and (!= msg.username "") (!= msg.password "")) then Rejected (chance otherwise)))
```

Handles one incoming protobuf message. Each `when` is one guarded case with exactly one `then` target.

### `when`

```text
(when (and (!= msg.username "") (!= msg.password "")) then Authenticated (chance 0.90))
(when (== msg.flour_kg 0) then IngredientConstrained (chance otherwise))
(when true then Done)
```

Adds a guard over the current message, then names the postcondition state. Use one `when` per case, and combine predicates with `and`, `or`, and `not`. A case without a condition is written as `(when true then Done)`.

### `chance otherwise`

```text
(when true then Accepted (chance 0.90))
(when true then Rejected (chance otherwise))
```

`chance otherwise` receives the remaining probability mass after numeric branch chances.

### `inbox`

```text
(inbox server
  (capacity 100))
```

Declares bounded FIFO capacity. Writes block when the inbox is full.

### `metric`

```text
(metric nominal_bytes_over_revision
  (chart line)
  (message ByteModel)
  (value msg.total_nominal_bytes)
  (window revision)
  (reducer sum))

(metric outcome_mix
  (chart pie)
  (message RenderedDocument)
  (group_by msg.format)
  (reducer count))
```

Declares a chart to compute from causal message traffic. Current output preserves these declarations in JSON, metrics text, and HTML. The simulator will use them as named reducers over traffic logs once MDP execution is implemented.

## CTL

Assertions live inside a conversation:

```text
(assert eventually_done
  (always (mustEventually (or Authenticated Rejected))))
```

Current readable aliases include:

- `possibly` / `risks` for `EF`
- `mustEventually` / `eventually` for `AF`
- `always` for `AG`
- `canPermanently` for `EF(EG ...)`
- `Until` / `until` for universal until
- `canUntil` for existential until

The checker renders mechanical English using:

- `A`: must
- `E`: may
- `F`: happen
- `G`: become

## Current Scope

The compiler currently:

- parses Lisp-form `.convspec` files only
- validates participants, message types, start states, and transition targets
- renders state diagrams, actor projections, interaction scenarios, metrics, JSON, and CTL checks
- parses enough protobuf syntax to discover top-level message names and fields

It does not yet evaluate guard expressions semantically or generate implementation code.
