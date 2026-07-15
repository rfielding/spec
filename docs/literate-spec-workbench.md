# Literate Spec Workbench

The target is an expressive engineering workbench, not a replacement terminal.

Codex is useful for editing code, but the medium is narrow: text turns, patches, and command output. Protocol and systems design need richer objects:

- interaction diagrams
- actor-local protocol projections
- state machines
- temporal logic proofs and counterexamples
- queueing and MDP views
- charts for latency, bytes, money, product mix, and load
- literate explanations that put the argument before the implementation

The workbench should make those objects native to the conversation.

## Literate Direction

The intended style is closer to literate programming: explain the system as a structured argument, then attach executable specifications, message schemas, checks, and generated implementation surfaces.

That reverses the usual code-comment relationship:

- code is not the primary artifact with comments attached
- the design document is the primary artifact
- code, protobuf messages, convspec conversations, diagrams, proofs, and metrics are executable blocks inside that document

For this project, a literate page should be able to say:

1. These are the actors.
2. These are the messages they exchange.
3. These are the legal scenarios.
4. These temporal claims pass.
5. These claims fail, with counterexample traces.
6. These queues saturate under this load.
7. These bytes and dollars moved through the system.
8. These callback surfaces are sufficient to implement each actor.

## Chat As Authoring Surface

The browser chat should become the place where specs are authored and revised.

Expected workflow:

1. Pick a conversation thread and model.
2. Edit `.convspec` and `.proto` files in the browser.
3. Ask design questions in the chat.
4. Compile deterministic evidence on every turn.
5. Render diagrams and charts in the evidence panel.
6. Let the LLM propose text, patches, or new questions against that evidence.
7. Save accepted file changes back to the repo.
8. Commit from the repo once the design has converged.

The LLM can help argue, summarize, and propose edits. It must not be the source of truth for diagrams, proofs, counterexamples, or measurements. Those artifacts come from deterministic compiler/checker/renderer code.

## Block Types

The chat protocol should grow into explicit blocks:

- `text`: prose response
- `patch`: proposed file edit
- `spec`: complete `.convspec` or `.proto` replacement
- `proof`: CTL/LTL result with exact formula, English rendering, status, and trace
- `diagram`: deterministic image artifact
- `chart`: deterministic metric visualization
- `trace`: counterexample or witness path
- `notebook`: literate design section containing text plus references to artifacts

The first server implementation already has `text` and deterministic `image` evidence. The next important step is structured `patch` blocks that the browser can preview, apply to the editor, compile, and save.

## Design Standard

The workbench should make high-quality engineering questions cheap:

- Show me the authentication sequence.
- Which actor callback surfaces are required?
- Does every reservation eventually commit, cancel, reject, or expire?
- Show me a counterexample if this property fails.
- How many bytes move over each actor pair?
- Which actor inbox fills and blocks senders?
- Which bakery day scenario wastes product?
- How much money entered, left, or remained unaccounted for?
- Render this as a literate implementation note.

The point is not to make a prettier chat UI. The point is to make the chat a front end for executable reasoning.
