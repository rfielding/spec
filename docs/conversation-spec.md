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

Actors are spec-wide. Each actor has one bounded FIFO inbox for the whole spec, which gives a single serialization order for message consumption across conversations. The actor `capacity` property says how many unread messages can wait before writes to that actor block. Capacity affects performance and possible concurrency, not the legal conversation behavior. The spec does not write `from` or `to` on handlers. Inside a conversation-local `(actor server ...)`, every `(on Message ...)` handles a `Message` received by `server`. If a source actor, return address, or actor instance matters, it belongs in the protobuf message.

Actor-wide declarations stay outside conversations:

- top-level `actor` declares the actor set for the whole spec
- top-level actor `capacity` declares unread-message capacity before writes block
- `reliability` declares actor availability assumptions
- root-level `assert` declares cross-conversation requirements; these are parsed but not evaluated yet

Conversation-local declarations are only the things that exist for one interaction diagram:

- `start` names the activation message that creates the conversation instance
- `actor` blocks redefine that actor's protocol states from scratch for this conversation
- `assert` and `metric` describe checks and views for that conversation

## Core Syntax

```text
(spec auth
  (import "auth.proto")
  (actor server (capacity 64))

  (include "auth_login.convspec")
)
```

Included conversation file:

```text
(conversation login
  (start server LoginConversationStarted Idle)
  ...)
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

### `include`

```text
(include "auth_login.convspec")
```

Loads one or more conversation forms from another `.convspec` file. Includes are resolved relative to the root spec file. Included files contribute conversations only; actors, capacity, imports, and reliability stay in the root spec.

The intended organization is to put includes after spec-wide context. As new requirements are identified, append new conversation includes to the end of the root spec.

### Top-Level `actor`

```text
(actor server
  (capacity 100))
```

Declares a logical actor role and its unread-message capacity. Actor instances, such as `truck-1` or `storefront-4`, should be modeled through message fields or later instance-binding syntax rather than hard-coded into every handler.

### `conversation`

```text
(conversation login
  (start server LoginConversationStarted Idle)
  ...)
```

Defines one protocol graph. The `start` form says that `server` consumes `LoginConversationStarted` from its spec-wide inbox to enter `Idle`, the initial state for this conversation. A version can be attached with `(version 2)`.

Conversation files are also useful as requirements artifacts before execution: they name the actors, states, messages, guards, probabilities, and expected terminal conditions in a stable form for human and LLM review.

### `actor`

```text
(actor server
  (state Idle ...))
```

Groups states owned by one actor. Handlers inside those states consume from that actor's inbox.

Actor blocks are conversation-local protocol projections. They do not define actor-wide resources; those belong at spec scope.

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
  (when (and (!= msg.username "") (!= msg.password "")) then Authenticated (chance 0.90)
    (send LoginResult
      (set authenticated true)))
  (when (and (!= msg.username "") (!= msg.password "")) then Rejected (chance otherwise)
    (send LoginResult
      (set authenticated false)
      (set reason "invalid credentials"))))
```

Handles one incoming protobuf message. Each `when` is one guarded case with exactly one `then` target.

The handler may declare `dwell_time_ms` for actor processing time. Bytes are derived from the protobuf message schema, so `bytes` is not valid convspec syntax.

### `when`

```text
(when (and (!= msg.username "") (!= msg.password "")) then Authenticated (chance 0.90))
(when (== msg.flour_kg 0) then IngredientConstrained (chance otherwise))
(when true then Done)
```

Adds a guard over the current message, then names the postcondition state. Use one `when` per case, and combine predicates with `and`, `or`, and `not`. A case without a condition is written as `(when true then Done)`.

### `send`

```text
(when (!= msg.username "") then Authenticated
  (send LoginResult
    (set authenticated true)
    (set conversation_id msg.conversation_id)))
```

Sends an observable protobuf message as part of the same guarded transition. `send` does not name a destination actor; routing belongs in the message payload when the protocol needs it, which keeps the same conversation usable for multiple actor instances.

Payload fields are specified with `set`. The compiler validates that the sent message type exists in the imported protobuf files and that each assigned field exists on that message. Field expressions are preserved as spec data, so they can describe dependencies such as copying from `msg`, deriving a value from received data, or filling a constant.

### `chance otherwise`

```text
(when true then Accepted (chance 0.90))
(when true then Rejected (chance otherwise))
```

`chance otherwise` receives the remaining probability mass after numeric branch chances.

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

Assertions usually live inside a conversation:

```text
(assert eventually_done
  (always (mustEventually (or Authenticated Rejected))))
```

Conversation assertions are evaluated against that conversation's state machine. Root-level assertions are reserved for requirements spanning conversations; the compiler parses and renders them, but does not yet evaluate cross-conversation CTL.

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
- validates actors, message types, start states, and transition targets
- renders state diagrams, actor projections, interaction scenarios, metrics, JSON, and CTL checks
- parses enough protobuf syntax to discover top-level message names and fields

It does not yet evaluate guard expressions semantically or generate implementation code.
