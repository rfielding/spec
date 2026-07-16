package convspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthCompilesToActorLocalConversation(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "auth" {
		t.Fatalf("spec name = %q, want auth", spec.Name)
	}
	if len(spec.Actors) != 1 || spec.Actors[0].Name != "server" || spec.Actors[0].Capacity != 64 {
		t.Fatalf("actors = %#v, want server capacity 64", spec.Actors)
	}
	if !spec.messageIndex["LoginRequest"] {
		t.Fatal("LoginRequest was not indexed from proto")
	}
	conversation := spec.Conversations[0]
	if conversation.StartActor != "server" || conversation.StartMessage != "LoginConversationStarted" || conversation.Start != "Idle" {
		t.Fatalf("start = actor %q message %q state %q", conversation.StartActor, conversation.StartMessage, conversation.Start)
	}
	state := conversation.States["Idle"]
	if state.Actor != "server" {
		t.Fatalf("Idle actor = %q, want server", state.Actor)
	}
	if len(state.Transitions) != 2 {
		t.Fatalf("Idle transitions = %d, want 2", len(state.Transitions))
	}
	for _, transition := range state.Transitions {
		if transition.Receiver != "server" {
			t.Fatalf("receiver = %q, want server", transition.Receiver)
		}
	}
	if conversation.States["Authenticated"].Terminal != "accept" {
		t.Fatal("Authenticated should be an accept state")
	}
}

func TestSpecModelCompilesWithMetricDeclarations(t *testing.T) {
	spec, err := ParseFile("../../examples/spec_model.convspec")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "spec_model" {
		t.Fatalf("spec name = %q, want spec_model", spec.Name)
	}
	conversation := spec.Conversations[0]
	if len(conversation.Metrics) != 3 {
		t.Fatalf("metrics = %d, want 3", len(conversation.Metrics))
	}
	if len(spec.Actors) != 4 {
		t.Fatalf("actors = %d, want 4", len(spec.Actors))
	}
	if len(spec.Asserts) != 1 || spec.Asserts[0].Name != "all_conversations_eventually_resolve" {
		t.Fatalf("root assertions = %#v", spec.Asserts)
	}
	if conversation.Metrics[0].Chart != "pie" || conversation.Metrics[0].Message != "RenderedDocument" {
		t.Fatalf("first metric = %#v", conversation.Metrics[0])
	}
	if !spec.messageIndex["ByteModel"] || !spec.messageIndex["MDPModel"] {
		t.Fatal("spec_model messages were not indexed from proto")
	}
}

func TestPaxosExampleCompiles(t *testing.T) {
	spec, err := ParseFile("../../examples/paxos.convspec")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "paxos" {
		t.Fatalf("spec name = %q, want paxos", spec.Name)
	}
	if len(spec.Actors) != 5 {
		t.Fatalf("actors = %d, want 5", len(spec.Actors))
	}
	if spec.Actors[1].Role != "paxos_acceptor" {
		t.Fatalf("acceptor role = %q, want paxos_acceptor", spec.Actors[1].Role)
	}
	if len(spec.Conversations) != 1 || spec.Conversations[0].Name != "basic_paxos" {
		t.Fatalf("conversations = %#v, want basic_paxos", spec.Conversations)
	}
	transition := spec.Conversations[0].States["Proposing"].Transitions[0]
	if len(transition.Sends) != 1 || transition.Sends[0].MessageType != "Prepare" {
		t.Fatalf("first transition sends = %#v, want Prepare", transition.Sends)
	}
	if !spec.messageIndex["Chosen"] {
		t.Fatal("Chosen message was not indexed from proto")
	}
}

func TestRootLevelAssertionIsReportedButNotEvaluated(t *testing.T) {
	spec, err := ParseFile("../../examples/spec_model.convspec")
	if err != nil {
		t.Fatal(err)
	}
	checks := EmitChecks(spec)
	if !strings.Contains(checks, "ERROR spec.all_conversations_eventually_resolve") {
		t.Fatalf("missing root assertion status:\n%s", checks)
	}
	if !strings.Contains(checks, "spec-level CTL assertions are parsed but not evaluated yet") {
		t.Fatalf("missing root assertion evaluation notice:\n%s", checks)
	}
}

func TestRootSpecIncludesConversationFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "temp.proto"), []byte(`syntax = "proto3";
package auth;
message Start {}
message Ping {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ping.convspec"), []byte(`(conversation ping
  (start server Start Idle)
  (actor server
    (state Idle
      (on Ping
        (when true then Done)))
    (state Done accept)))`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "temp.convspec"), []byte(`(spec auth
  (import "temp.proto")
  (include "ping.convspec")
  (actor server (capacity 8)))`), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := ParseFile(filepath.Join(dir, "temp.convspec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Conversations) != 1 || spec.Conversations[0].Name != "ping" {
		t.Fatalf("included conversations = %#v", spec.Conversations)
	}
	if len(spec.Includes) != 1 || spec.Includes[0] != "ping.convspec" {
		t.Fatalf("includes = %#v", spec.Includes)
	}
}

func TestActorCanDeclareRoleAndParams(t *testing.T) {
	spec := parseTempSpec(t, `syntax = "proto3";
package cluster;
message Start {}
message Prepare {}`, `(spec cluster
  (import "temp.proto")
  (actor acceptor_1
    (role paxos_acceptor)
    (capacity 32)
    (param zone "us-east-1a")
    (param weight 2))
  (conversation prepare
    (start acceptor_1 Start Waiting)
    (actor acceptor_1
      (state Waiting
        (on Prepare
          (when true then Done)))
      (state Done accept))))`)
	if len(spec.Actors) != 1 {
		t.Fatalf("actors = %#v, want one actor", spec.Actors)
	}
	actor := spec.Actors[0]
	if actor.Name != "acceptor_1" || actor.Role != "paxos_acceptor" || actor.Capacity != 32 {
		t.Fatalf("actor = %#v, want acceptor_1 role paxos_acceptor capacity 32", actor)
	}
	if len(actor.Params) != 2 {
		t.Fatalf("params = %#v, want two params", actor.Params)
	}
	if actor.Params[0].Name != "zone" || actor.Params[0].Value != "\"us-east-1a\"" {
		t.Fatalf("first param = %#v, want quoted zone", actor.Params[0])
	}
	if actor.Params[1].Name != "weight" || actor.Params[1].Value != "2" {
		t.Fatalf("second param = %#v, want weight=2", actor.Params[1])
	}
}

func TestIncludedFileRejectsActorDeclarations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "temp.proto"), []byte(`syntax = "proto3";
package auth;
message Start {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.convspec"), []byte(`(spec bad
  (actor server (capacity 8)))`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "temp.convspec"), []byte(`(spec auth
  (import "temp.proto")
  (include "bad.convspec")
  (actor server (capacity 8)))`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseFile(filepath.Join(dir, "temp.convspec"))
	if err == nil || !strings.Contains(err.Error(), "included spec fragments may only contain conversation forms") {
		t.Fatalf("expected included actor declaration error, got %v", err)
	}
}

func TestLispRejectsConversationLocalInbox(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}
message Ping {}`, `(spec auth
  (import "temp.proto")
  (actor server (capacity 8))
  (conversation ping
    (start server Start Idle)
    (inbox server (capacity 1))
    (actor server
      (state Idle
        (on Ping
          (when true then Done)))
      (state Done accept))))`)
	if err == nil || !strings.Contains(err.Error(), "inbox is not a conversation form") {
		t.Fatalf("expected conversation inbox error, got %v", err)
	}
}

func TestLispRejectsParticipants(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}`, `(spec auth
  (import "temp.proto")
  (participants server)
  (conversation ping
    (start server Start Idle)
    (actor server
      (state Idle accept))))`)
	if err == nil || !strings.Contains(err.Error(), "participants is no longer a top-level form") {
		t.Fatalf("expected participants error, got %v", err)
	}
}

func TestLispRejectsTopLevelInbox(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}`, `(spec auth
  (import "temp.proto")
  (inbox server (capacity 8))
  (conversation ping
    (start server Start Idle)
    (actor server
      (state Idle accept))))`)
	if err == nil || !strings.Contains(err.Error(), "use (actor <name> (capacity <n>))") {
		t.Fatalf("expected top-level inbox error, got %v", err)
	}
}

func TestLispRejectsConversationLocalQueue(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}
message Ping {}`, `(spec auth
  (import "temp.proto")
  (actor server (capacity 8))
  (conversation ping
    (start server Start Idle)
    (queue work (capacity 1))
    (actor server
      (state Idle
        (on Ping
          (when true then Done)))
      (state Done accept))))`)
	if err == nil || !strings.Contains(err.Error(), "queue is not a conversation form") {
		t.Fatalf("expected conversation queue error, got %v", err)
	}
}

func TestLispRejectsHandlerQueue(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}
message Ping {}`, `(spec auth
  (import "temp.proto")
  (actor server (capacity 8))
  (conversation ping
    (start server Start Idle)
    (actor server
      (state Idle
        (on Ping
          (queue work)
          (when true then Done)))
      (state Done accept))))`)
	if err == nil || !strings.Contains(err.Error(), "queue is not a handler property") {
		t.Fatalf("expected handler queue error, got %v", err)
	}
}

func TestLispRejectsHandlerBytes(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}
message Ping {}`, `(spec auth
  (import "temp.proto")
  (actor server (capacity 8))
  (conversation ping
    (start server Start Idle)
    (actor server
      (state Idle
        (on Ping
          (bytes 12)
          (when true then Done)))
      (state Done accept))))`)
	if err == nil || !strings.Contains(err.Error(), "bytes is derived from protobuf serialization") {
		t.Fatalf("expected handler bytes error, got %v", err)
	}
}

func TestLispRejectsLatencyAlias(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}
message Ping {}`, `(spec auth
  (import "temp.proto")
  (actor server (capacity 8))
  (conversation ping
    (start server Start Idle)
    (actor server
      (state Idle
        (on Ping
          (latency_ms 12)
          (when true then Done)))
      (state Done accept))))`)
	if err == nil || !strings.Contains(err.Error(), "use dwell_time_ms") {
		t.Fatalf("expected latency alias error, got %v", err)
	}
}

func TestLispRejectsAbstractStartState(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package auth;
message Start {}`, `(spec auth
  (import "temp.proto")
  (actor server (capacity 8))
  (conversation ping
    (start Idle)
    (actor server
      (state Idle accept))))`)
	if err == nil || !strings.Contains(err.Error(), "expected: (start <actor> <MessageType> <state>)") {
		t.Fatalf("expected explicit start message error, got %v", err)
	}
}

func TestDOTMarksTerminalStates(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	diagram := EmitDOT(spec)
	for _, want := range []string{
		`rankdir="TB"`,
		`label="login state machine"`,
		`"Authenticated" [label="Authenticated\nstate_is authenticated\nstate_is terminal", shape="doublecircle"`,
		`"Rejected" [label="Rejected\nstate_is rejected\nstate_is terminal", shape="doublecircle"`,
		`server receives LoginRequest`,
		`bgcolor="#0f172a"`,
	} {
		if !strings.Contains(diagram, want) {
			t.Fatalf("DOT missing %q:\n%s", want, diagram)
		}
	}
	if strings.Contains(diagram, "(from") || strings.Contains(diagram, "(to") {
		t.Fatalf("DOT should not expose endpoint syntax:\n%s", diagram)
	}
}

func TestSequenceMermaidEnumeratesTerminalPaths(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	diagrams := EmitMermaidSequences(spec)
	if got := strings.Count(diagrams, "sequenceDiagram"); got != 2 {
		t.Fatalf("sequence diagrams = %d, want 2:\n%s", got, diagrams)
	}
	if !strings.Contains(diagrams, "LoginRequest") || !strings.Contains(diagrams, "Authenticated") || !strings.Contains(diagrams, "Rejected") {
		t.Fatalf("sequence diagrams missing auth outcomes:\n%s", diagrams)
	}
}

func TestJSONContainsMessagesAndConversations(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := EmitJSON(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"messages"`) || !strings.Contains(payload, `"LoginRequest"`) {
		t.Fatalf("JSON missing messages:\n%s", payload)
	}
}

func TestHTMLRendersStateAndTerminalPathDiagrams(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	page, err := EmitHTML(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<!doctype html>",
		"auth",
		"CTL Checks",
		"eventually_done",
		"PASS",
		"State machine",
		"Actor Protocol Projections",
		"Interaction Scenarios (2)",
		`<img src=`,
		"data:image/png;base64,",
		"data:image/svg+xml;base64,",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
}

func TestActorProjectionLabelsReceives(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	diagram := dotActorConversation(spec.Conversations[0], "server")
	if !strings.Contains(diagram, "server protocol projection") || !strings.Contains(diagram, "receive LoginRequest") {
		t.Fatalf("actor projection missing receive handler:\n%s", diagram)
	}
	if strings.Contains(diagram, " from ") || strings.Contains(diagram, " to ") {
		t.Fatalf("actor projection should not mention sender/recipient endpoints:\n%s", diagram)
	}
}

func TestMetricsFromActorLocalChanceOtherwise(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	metrics := ComputeMetrics(spec)
	conversation := metrics.Conversations[0]
	if !conversation.HasQuantities {
		t.Fatal("expected chance metrics")
	}
	if len(conversation.Scenarios) != 2 {
		t.Fatalf("scenario metrics = %d, want 2", len(conversation.Scenarios))
	}
	if conversation.Scenarios[0].Probability < 0.899 || conversation.Scenarios[0].Probability > 0.901 {
		t.Fatalf("first scenario probability = %.3f, want .900", conversation.Scenarios[0].Probability)
	}
	if conversation.Scenarios[1].Probability < 0.099 || conversation.Scenarios[1].Probability > 0.101 {
		t.Fatalf("otherwise probability = %.3f, want .100", conversation.Scenarios[1].Probability)
	}
}

func TestLispGuardBranchesShareOneObservedMessage(t *testing.T) {
	spec := parseTempSpec(t, `syntax = "proto3";
package branch;
message Start {}
message Draw {
  string day_id = 1;
  uint32 flour_kg = 2;
}`, `(spec branch
  (import "temp.proto")
  (actor bakery (capacity 8))
  (conversation draw
    (start bakery Start Waiting)
    (actor bakery
      (state Waiting
        (on Draw
          (when (and (!= msg.day_id "") (!= msg.flour_kg 0)) then DoughMixing (chance 0.88))
          (when (and (!= msg.day_id "") (== msg.flour_kg 0)) then IngredientConstrained (chance otherwise))))
      (state DoughMixing accept)
      (state IngredientConstrained accept))))`)
	transitions := spec.Conversations[0].States["Waiting"].Transitions
	if len(transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(transitions))
	}
	if transitions[0].Guard != `msg.day_id != "" and msg.flour_kg != 0` {
		t.Fatalf("first guard = %q", transitions[0].Guard)
	}
	if transitions[1].Guard != `msg.day_id != "" and msg.flour_kg == 0` {
		t.Fatalf("second guard = %q", transitions[1].Guard)
	}
	if transitions[0].Chance == nil || *transitions[0].Chance != 0.88 || !transitions[1].Otherwise {
		t.Fatalf("branch chances = %#v", transitions)
	}
}

func TestLispRejectsBareThen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "temp.proto"), []byte(`syntax = "proto3";
package auth;
message LoginStarted {}
message LoginRequest {
  string username = 1;
  string password = 2;
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "temp.convspec"), []byte(`(spec auth
  (import "temp.proto")
  (actor server (capacity 8))
  (conversation login
    (start server LoginStarted Idle)
    (actor server
      (state Idle
        (on LoginRequest
          (when (!= msg.username "") then Authenticated)
          (then Authenticated)))
      (state Authenticated accept))))`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseFile(filepath.Join(dir, "temp.convspec"))
	if err == nil || !strings.Contains(err.Error(), "unexpected transition form: then") {
		t.Fatalf("expected bare then error, got %v", err)
	}
}

func TestLispWhenTrueMeansUnconditional(t *testing.T) {
	spec := parseTempSpec(t, `syntax = "proto3";
package tick;
message Start {}
message Tick {}`, `(spec tick
  (import "temp.proto")
  (actor worker (capacity 8))
  (conversation tick
    (start worker Start Waiting)
    (actor worker
      (state Waiting
        (on Tick
          (when true then Done)))
      (state Done accept))))`)
	transitions := spec.Conversations[0].States["Waiting"].Transitions
	if len(transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(transitions))
	}
	if transitions[0].Guard != "true" || transitions[0].Target != "Done" {
		t.Fatalf("transition = %#v, want true guard to Done", transitions[0])
	}
}

func TestLispWhenCanSendPayload(t *testing.T) {
	spec := parseTempSpec(t, `syntax = "proto3";
package tick;
message Start {}
message Tick {
  string id = 1;
}
message Tock {
  string id = 1;
  string proof = 2;
}`, `(spec tick
  (import "temp.proto")
  (actor worker (capacity 8))
  (conversation tick
    (start worker Start Waiting)
    (actor worker
      (state Waiting
        (on Tick
          (when (!= msg.id "") then Done
            (send Tock
              (set id msg.id)
              (set proof (derive msg.id))))))
      (state Done accept))))`)
	transition := spec.Conversations[0].States["Waiting"].Transitions[0]
	if len(transition.Sends) != 1 {
		t.Fatalf("sends = %#v, want one send", transition.Sends)
	}
	sent := transition.Sends[0]
	if sent.MessageType != "Tock" || len(sent.Fields) != 2 {
		t.Fatalf("sent = %#v, want Tock with two fields", sent)
	}
	if sent.Fields[0].Name != "id" || sent.Fields[0].Value != "msg.id" {
		t.Fatalf("first field = %#v, want id=msg.id", sent.Fields[0])
	}
	if sent.Fields[1].Name != "proof" || sent.Fields[1].Value != "derive(msg.id)" {
		t.Fatalf("second field = %#v, want proof=derive(msg.id)", sent.Fields[1])
	}
}

func TestLispSendValidatesKnownMessageAndFields(t *testing.T) {
	_, err := parseTempSpecErr(t, `syntax = "proto3";
package tick;
message Start {}
message Tick {}
message Tock {
  string id = 1;
}`, `(spec tick
  (import "temp.proto")
  (actor worker (capacity 8))
  (conversation tick
    (start worker Start Waiting)
    (actor worker
      (state Waiting
        (on Tick
          (when true then Done
            (send Tock (set missing msg.id))
            (send Unknown))))
      (state Done accept))))`)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "sent message Tock has unknown field missing") {
		t.Fatalf("missing unknown field error: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown sent message Unknown") {
		t.Fatalf("missing unknown sent message error: %v", err)
	}
}

func TestReliabilityValidationRejectsUnknownActor(t *testing.T) {
	spec := &Spec{
		Actors:       []ActorSpec{{Name: "client", Capacity: 1}},
		Reliability:  []ReliabilitySpec{{Actor: "server", Availability: 0.99}},
		messageIndex: map[string]bool{},
	}
	err := Validate(spec)
	if err == nil || !strings.Contains(err.Error(), "unknown actor server") {
		t.Fatalf("expected unknown actor reliability error, got %v", err)
	}
}

func TestCTLAssertionsEvaluateAgainstObservableStates(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	results := EvaluateAssertions(spec)
	if len(results) != 3 {
		t.Fatalf("assertion results = %d, want 3", len(results))
	}
	for _, result := range results {
		if result.Error != "" {
			t.Fatalf("%s had parse/eval error: %s", result.Name, result.Error)
		}
		if !result.Pass {
			t.Fatalf("%s did not pass: %#v", result.Name, result)
		}
	}
}

func TestCTLReadableAliases(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	spec.Conversations[0].Asserts = append(spec.Conversations[0].Asserts,
		Assertion{Name: "can_stay_rejected", Formula: "can_stabilize(Rejected)"},
		Assertion{Name: "risks_rejection", Formula: "risks(Rejected)"},
		Assertion{Name: "must_eventually_terminal", Formula: "mustEventually(Authenticated or Rejected)"},
		Assertion{Name: "can_permanently_rejected", Formula: "canPermanently(Rejected)"},
	)
	results := EvaluateAssertions(spec)
	for _, result := range results[len(results)-4:] {
		if result.Error != "" {
			t.Fatal(result.Error)
		}
		if !result.Pass {
			t.Fatalf("expected readable alias formula to pass: %#v", result)
		}
	}
}

func TestCTLFormulaEnglishDescription(t *testing.T) {
	cases := map[string]string{
		"AF(sickness -> or(AF(well), AG(dead)))": "must happen sickness implies (must happen well or must become dead)",
		"canPermanently(dead)":                   "may happen may become dead",
		"EG(dead)":                               "may become dead",
		"(and not(well) dead)":                   "not well and dead",
		"alive Until dead":                       "must alive until dead",
		"canUntil(alive, dead)":                  "may alive until dead",
		"EF(not(well) -> EU(well, virus))":       "may happen not well implies may well until virus",
		"AG(alive -> AU(alive, dead))":           "must become alive implies must alive until dead",
	}
	for formula, want := range cases {
		expr, err := parseCTL(formula)
		if err != nil {
			t.Fatal(err)
		}
		if got := describeCTL(expr); got != want {
			t.Fatalf("%s english = %q, want %q", formula, got, want)
		}
	}
}

func TestCTLUntilEvaluatesSemiPermanentPropositions(t *testing.T) {
	conversation := Conversation{
		Name:  "life",
		Start: "Healthy",
		Order: []string{"Healthy", "Sick", "Dead"},
		States: map[string]State{
			"Healthy": {Name: "Healthy", StateIs: []string{"alive"}, Transitions: []Transition{{Target: "Sick"}}},
			"Sick":    {Name: "Sick", StateIs: []string{"alive", "sickness"}, Transitions: []Transition{{Target: "Dead"}}},
			"Dead":    {Name: "Dead", Terminal: "accept", StateIs: []string{"dead"}},
		},
		Asserts: []Assertion{
			{Name: "alive_until_dead", Formula: "alive Until dead"},
			{Name: "not_both_alive_and_dead", Formula: "always(!(alive and dead))"},
			{Name: "sickness_can_end_in_death", Formula: "possibly(sickness canUntil dead)"},
		},
	}
	results := EvaluateAssertions(&Spec{Conversations: []Conversation{conversation}})
	for _, result := range results {
		if result.Error != "" {
			t.Fatalf("%s errored: %s", result.Name, result.Error)
		}
		if !result.Pass {
			t.Fatalf("%s failed: %#v", result.Name, result)
		}
	}
}

func TestEmitChecks(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	out := EmitChecks(spec)
	for _, want := range []string{
		"PASS login.eventually_done",
		"PASS login.success_possible",
		"PASS login.rejection_possible",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("checks output missing %q:\n%s", want, out)
		}
	}
}

func TestCTLCanFail(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	spec.Conversations[0].Asserts = append(spec.Conversations[0].Asserts, Assertion{
		Name:    "authenticated_always",
		Formula: "always(Authenticated)",
	})
	results := EvaluateAssertions(spec)
	last := results[len(results)-1]
	if last.Error != "" {
		t.Fatal(last.Error)
	}
	if last.Pass {
		t.Fatalf("expected %s to fail", last.Name)
	}
}

func parseTempSpec(t *testing.T, protoText string, specText string) *Spec {
	t.Helper()
	spec, err := parseTempSpecErr(t, protoText, specText)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func parseTempSpecErr(t *testing.T, protoText string, specText string) (*Spec, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "temp.proto"), []byte(protoText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "temp.convspec"), []byte(specText), 0o644); err != nil {
		t.Fatal(err)
	}
	return ParseFile(filepath.Join(dir, "temp.convspec"))
}
