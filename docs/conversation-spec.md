# Conversation Spec

## Purpose

Protocol design usually mixes two very different concerns:

1. atomic wire format
2. legal multi-message interaction

Protobuf is strong at the first concern. This spec layer handles the second.

The conversation spec is not another serializer. It is a protocol contract over protobuf messages.

## Design Goals

- Keep `.proto` files simple and reusable.
- Express valid conversations as explicit state machines.
- Allow constraints over message contents without duplicating protobuf schema.
- Make trace validation and code generation possible.
- Support request-response, streaming, retries, cancellations, and branching flows.
- Make the observable protocol semantics explicit enough to compile into a Kripke structure for CTL.

## Conceptual Model

A conversation spec compiles to a labeled transition system.

- Nodes are conversation states.
- Edges are message events.
- Each event has:
  - sender
  - receiver
  - protobuf message type
  - optional guard
  - optional bindings
  - target state

A valid conversation trace is a path through that machine starting from `start` and ending in an `accept` or `reject` state.

For temporal verification, that transition system is then reified as a Kripke structure:

- each reachable protocol state becomes a Kripke node
- each legal observed message step becomes a transition
- each node is labeled with atomic propositions derived from state, actor obligations, message history, and version

That is the object checked by CTL.

## Separation of Concerns

What stays in protobuf:

- field names and numbers
- scalar and nested types
- enums
- oneof layout
- binary and JSON serialization

What moves to conversation spec:

- who may send what
- in what order
- when a message is allowed
- how one message relates to earlier messages
- whether retries are allowed
- when the conversation is complete or failed
- which protocol facts are observable as atomic propositions

## Observable Semantics

If your goal is model checking, the spec cannot stop at “valid trace” parsing. It must define the observable state machine.

A conversation instance therefore has two layers:

1. wire layer
2. observable protocol layer

The wire layer is a stream of versioned protobuf messages. The observable protocol layer is the abstract state seen by the model checker.

Minimal observable state:

- current protocol state name
- negotiated or declared wire version
- correlation key
- bound messages relevant to future guards
- enabled outgoing transitions
- derived propositions

Example derived propositions:

- `pending`
- `reserved`
- `confirmed`
- `cancelled`
- `failed`
- `awaiting_supplier`
- `version_v1`
- `version_v2`

The spec should let these propositions be named directly, rather than inferred indirectly from implementation code.

## Core Language

This is a first-pass DSL, optimized for readability over minimal syntax.

```text
spec auth

import "auth.proto"

participants
  client
  server

conversation login {
  start Idle

  state Idle {
    on client -> server LoginRequest
      bind req
      when req.username != ""
      then AwaitDecision
  }

  state AwaitDecision {
    on server -> client LoginAccepted
      when message.username == req.username
      then Authenticated

    on server -> client LoginRejected
      when message.username == req.username
      then Done
  }

  state Authenticated accept
  state Done accept
}
```

## Syntax Elements

### `spec`

Names the module.

```text
spec auth
```

### `import`

Loads protobuf definitions and exposes message symbols.

```text
import "auth.proto"
```

Implementation note: this should resolve through `protoc` descriptors or a descriptor set, not by inventing a second protobuf parser.

### `participants`

Declares the conversation actors.

```text
participants
  client
  server
```

These are logical roles, not concrete processes. A runtime can later map roles to services, sockets, agents, or users.

### `conversation`

Defines a named protocol over imported message types.

```text
conversation login { ... }
```

One file may define multiple conversations if they share imports and participants.

Recommended extension for versioned wire protocols:

```text
conversation reservation version 1 { ... }
conversation reservation version 2 { ... }
```

This keeps protobuf message compatibility concerns separate from behavioral compatibility concerns. You can reuse the same messages across versions while changing legal flow, or vice versa.

### `state`

Defines a node in the protocol graph.

Variants:

- `state X { ... }`
- `state X accept`
- `state X reject`

`accept` means a valid terminal state. `reject` means the protocol intentionally reaches a failure terminal state.

Recommended extension for CTL labeling:

```text
state Held {
  state_is pending
  state_is hold_active
}
```

`state_is` does not send a protobuf message. It labels the current protocol state with a boolean fact that is observable to the model checker. While the conversation is in `Held`, both `pending` and `hold_active` are true. After leaving that state, they are no longer true unless the next state also declares the same facts.

### `on`

Defines a transition triggered by a protobuf message.

```text
on client -> server LoginRequest
```

Semantics:

- the next trace event must be a `LoginRequest`
- it must be sent by `client`
- it must be received by `server`

### `bind`

Names the current observed message for later guards and correlations.

```text
bind req
```

This does not assign or set fields. The message has already been sent on the wire by the actor named in the transition. `bind req` means “refer to this observed message instance as `req` later.”

For example:

```text
on bakers -> bakery BakersArrive
  bind arrival
  when arrival.day_id != ""
  then Planning

on bakery -> inventory DailyBakePlan
  bind plan
  when plan.day_id == arrival.day_id
  then InventoryCheck
```

Here `arrival.day_id` is read from the earlier `BakersArrive` message. The later `DailyBakePlan` is legal only when its `day_id` matches that earlier observed value.

### `when`

Adds a boolean guard.

```text
when req.username != ""
when message.username == req.username
```

Reserved identifiers:

- `message`: the current incoming message on this transition
- any previous `bind` name

First implementation can support a deliberately small expression language:

- equality and inequality
- boolean `and` / `or`
- enum literals
- numeric comparison
- string emptiness
- field presence

### `then`

Names the postcondition state reached after the message has been observed and the guards are satisfied.

```text
then AwaitDecision
then Rejected chance 0.12
```

`then` is not an imperative jump that sends a message. The message has already been named by the `on actor -> actor MessageType` line. `then` says which state the conversation is in after that observation. If multiple outgoing observations are possible from a state, `chance` belongs on the `then` outcome.

### `state_is`

Labels the current protocol state with propositions used by the model checker.

```text
state Confirmed accept {
  state_is reserved
  state_is confirmed
}
```

This is how temporal assertions talk about states:

```text
assert hold_settles: always(hold_active -> mustEventually(confirmed or cancelled or expired))
```

Here `hold_active`, `confirmed`, `cancelled`, and `expired` are not messages. They are state labels declared with `state_is`.

The first implementation restricts `state_is` to identifiers, not arbitrary formulas.

### `version`

Declares the wire-protocol version represented by a conversation.

```text
conversation reservation version 2 { ... }
```

The compiled Kripke nodes should then also carry a proposition such as `version_2`.

## Example

Protobuf:

```proto
syntax = "proto3";

package auth;

message LoginRequest {
  string username = 1;
  string password = 2;
}

message LoginAccepted {
  string username = 1;
  string session_id = 2;
}

message LoginRejected {
  string username = 1;
  string reason = 2;
}
```

Conversation spec:

```text
spec auth

import "auth.proto"

participants
  client
  server

conversation login {
  start Idle

  state Idle {
    on client -> server LoginRequest
      bind req
      when req.username != ""
      when req.password != ""
      then AwaitDecision
  }

  state AwaitDecision {
    on server -> client LoginAccepted
      when message.username == req.username
      when message.session_id != ""
      then Authenticated

    on server -> client LoginRejected
      when message.username == req.username
      when message.reason != ""
      then Rejected
  }

  state Authenticated accept
  state Rejected accept
}
```

Valid trace:

1. `client -> server LoginRequest{username="alice", password="secret"}`
2. `server -> client LoginAccepted{username="alice", session_id="s123"}`

Invalid trace examples:

1. `server -> client LoginAccepted` before any `LoginRequest`
2. `LoginAccepted.username != LoginRequest.username`
3. two `LoginRequest` messages in a row without an explicit retry transition

## Reservation Example

This is closer to your target use case: multiple actors, a versioned wire protocol, and a conversation whose observable behavior is intended for CTL.

Actors:

- `client`
- `broker`
- `supplier`

Wire protocol versions:

- version 1: hold, confirm, cancel
- version 2: adds explicit supplier refusal and hold expiry

Behaviorally, the protocol should distinguish:

- request submitted
- hold pending at supplier
- hold granted
- confirmed
- cancelled
- expired
- rejected

Those become propositions in the compiled Kripke structure.

Example CTL properties:

- `AG (confirmed -> AF terminal)`
- `AG (hold_active -> AX (!confirmed or hold_active))`
- `AG (submitted -> AF (confirmed or cancelled or rejected or expired))`
- `AG !(confirmed and cancelled)`

The important point is that CTL runs over the abstract protocol states, not over arbitrary raw message payload snapshots.

## Versioning Strategy

Treat versioning as a first-class protocol concern.

The wire protocol is versioned, so the conversation spec should support at least two patterns:

1. explicit per-version conversations
2. version negotiation as an initial subprotocol

Pattern 1 is simpler:

```text
conversation reservation version 1 { ... }
conversation reservation version 2 { ... }
```

Pattern 2 is useful if negotiation itself is observable:

```text
conversation session_setup {
  start Unnegotiated

  state Unnegotiated {
    on client -> broker Hello
      bind hello
      then AwaitVersionChoice
  }

  state AwaitVersionChoice {
    on broker -> client HelloAck
      when message.selected_version in [1, 2]
      then Negotiated
  }
}
```

Then later conversations can be parameterized by `selected_version`.

For a first implementation, prefer pattern 1. It makes the state space smaller and the CTL story cleaner.

## Multi-Actor Reservations

A reservation system naturally has more than two actors. That changes the model in an important way: a conversation transition is no longer just request-response between fixed endpoints. It becomes a routed event in a protocol graph.

Example:

```text
on client -> broker CreateReservation
on broker -> supplier HoldRequest
on supplier -> broker HoldGranted
on broker -> client ReservationHeld
```

This is still a normal labeled transition system. The only extra requirement is that participant roles are part of the transition label and can appear in propositions if needed, such as `awaiting_supplier`.

## CTL Compilation

Compile each conversation into a Kripke structure:

```text
K = (S, R, L, s0)
```

- `S`: reachable observable protocol states
- `R`: legal next-step transitions induced by allowed messages
- `L`: atomic propositions true at each state
- `s0`: the start state

Construction sketch:

1. Start from the conversation `start` state.
2. Expand all legal transitions.
3. Carry forward bound variables needed by future guards.
4. Canonicalize equivalent observable states.
5. Label each state with:
   - state-name propositions
   - version propositions
   - terminal / nonterminal propositions
   - user-declared `state_is` propositions
6. Add self-loops on terminal states if your model checker expects total transition relations.

This last point matters for CTL semantics. Many tools assume every state has at least one successor.

## What Should Count As Observable

Be strict here. Only expose protocol facts that are stable and semantically meaningful.

Good observable propositions:

- `submitted`
- `hold_active`
- `confirmed`
- `cancelled`
- `rejected`
- `expired`
- `terminal`
- `version_2`

Bad observable propositions:

- raw password-like fields
- every protobuf scalar by default
- transport-specific sequence numbers unless they matter to the protocol

The model checker should reason over protocol semantics, not packet trivia.

## Validation Model

Given a trace of observed messages:

1. identify candidate conversation instances
2. assign roles and correlation ids
3. step the automaton for each message
4. evaluate guards using the current message plus bound history
5. accept if the trace reaches an `accept` state without illegal transitions

For model checking, validation is not enough. You also need exhaustive reachability over all legal conversations admitted by the spec, not just one concrete trace.

Validation failure should report:

- current state
- unexpected message type
- direction mismatch
- guard that failed
- source span in `.convspec`

## Implementation Strategy

A practical implementation path:

1. Parse `.convspec` into an AST.
2. Load protobuf descriptors from `protoc`.
3. Resolve message symbols against descriptors.
4. Type-check field paths used in guards.
5. Compile each conversation to an observable state machine.
6. Project that machine into a Kripke structure.
7. Validate message traces against the same transition relation.
8. Export the Kripke structure to your CTL toolchain.

Good internal representation:

```text
Conversation
  name
  participants[]
  states[]

State
  name
  terminal_kind
  emitted_props[]
  transitions[]

Transition
  from_role
  to_role
  message_type
  bind_name?
  guards[]
  target_state

ObservableState
  protocol_state_name
  version
  bindings[]
  propositions[]
```

## Why This Boundary Works

This avoids turning protobuf into a workflow language, and avoids turning the workflow language into a serializer.

That separation gives you:

- reusable message types across multiple protocols
- explicit protocol review
- trace validation
- protocol-aware test generation
- possible codegen for typed client and server stubs with state awareness
- a clean compilation path into CTL model checking

## Naming

Reasonable file naming:

- `auth.proto`
- `auth.convspec`

If you want multiple protocol layers over one message schema:

- `messages.proto`
- `login.convspec`
- `password_reset.convspec`
- `session_resume.convspec`

If you want explicit behavioral versions:

- `reservation.v1.convspec`
- `reservation.v2.convspec`

## Recommendation

Start with these constraints:

- one active conversation instance per correlation key
- deterministic transitions within a state
- no parallel regions
- no user-defined functions in guards
- no mutation beyond named message bindings
- explicit proposition labels via `state_is`
- one conversation definition per wire-protocol version

That keeps the first validator simple and makes later extension easier.
