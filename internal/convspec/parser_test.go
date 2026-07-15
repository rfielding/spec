package convspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthCompilesToConversationGraph(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "auth" {
		t.Fatalf("spec name = %q, want auth", spec.Name)
	}
	if got := strings.Join(spec.Participants, ","); got != "client,server" {
		t.Fatalf("participants = %q", got)
	}
	if !spec.messageIndex["LoginRequest"] {
		t.Fatal("LoginRequest was not indexed from proto")
	}
	conversation := spec.Conversations[0]
	if conversation.Start != "Idle" {
		t.Fatalf("start = %q, want Idle", conversation.Start)
	}
	if conversation.States["Authenticated"].Terminal != "accept" {
		t.Fatal("Authenticated should be an accept state")
	}
	if conversation.States["Idle"].Transitions[0].Target != "AwaitDecision" {
		t.Fatal("Idle transition should target AwaitDecision")
	}
}

func TestReservationMermaidIncludesAllBranches(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	diagram := EmitMermaid(spec)
	required := []string{
		"conversation reservation_v2",
		"Idle --> AwaitSupplierHold",
		"SupplierEvaluating --> Held",
		"SupplierEvaluating --> Rejected",
		"Held --> Cancelled",
		"AwaitConfirmation --> Confirmed",
		"AwaitConfirmation --> Cancelled",
	}
	for _, want := range required {
		if !strings.Contains(diagram, want) {
			t.Fatalf("diagram missing %q:\n%s", want, diagram)
		}
	}
}

func TestDOTMarksTerminalStates(t *testing.T) {
	spec, err := ParseFile("../../examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	diagram := EmitDOT(spec)
	if !strings.Contains(diagram, `rankdir="TB"`) {
		t.Fatal("DOT should render top-to-bottom")
	}
	if !strings.Contains(diagram, `label="login state machine"`) {
		t.Fatal("DOT should include graph title")
	}
	if !strings.Contains(diagram, `"Authenticated" [label="Authenticated", shape="doublecircle"`) {
		t.Fatal("DOT did not mark Authenticated as accept terminal")
	}
	if strings.Contains(diagram, `Authenticated\naccept`) {
		t.Fatal("DOT should not repeat accept marker in terminal node labels")
	}
	if !strings.Contains(diagram, `"Rejected" [label="Rejected", shape="doublecircle"`) {
		t.Fatal("DOT did not mark Rejected as accept terminal")
	}
	if !strings.Contains(diagram, `bgcolor="#0f172a"`) {
		t.Fatal("DOT should use dark-mode background")
	}
}

func TestInteractionDiagramOmitsAcceptText(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	path := enumeratePaths(spec.Conversations[0])[0]
	svg := interactionSVG(spec.Conversations[0], 1, path)
	if !strings.Contains(svg, "outcome: Confirmed") {
		t.Fatalf("interaction SVG missing outcome label:\n%s", svg)
	}
	if strings.Contains(svg, "Confirmed accept") || strings.Contains(svg, "terminal:") {
		t.Fatalf("interaction SVG should not include terminal accept marker:\n%s", svg)
	}
}

func TestInteractionDiagramLabelsFocusOnProtoMessage(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	path := enumeratePaths(spec.Conversations[0])[0]
	svg := interactionSVG(spec.Conversations[0], 1, path)
	if !strings.Contains(svg, ">CreateReservation<") {
		t.Fatalf("interaction SVG should label message by protobuf type:\n%s", svg)
	}
	if strings.Contains(svg, "client → broker: CreateReservation") {
		t.Fatalf("interaction SVG should not repeat sender/receiver in message labels:\n%s", svg)
	}
	if !strings.Contains(svg, ">ReservationConfirmed<") {
		t.Fatalf("interaction SVG should include protobuf response message name:\n%s", svg)
	}
	if strings.Contains(svg, `!= &#34;&#34;`) || strings.Contains(svg, `!= ""`) {
		t.Fatalf("interaction SVG should hide default-value presence guards:\n%s", svg)
	}
	if !strings.Contains(svg, "msg.protocol_version == &#34;2&#34;") {
		t.Fatalf("interaction SVG should keep non-default guards:\n%s", svg)
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
	if !strings.Contains(diagrams, "LoginAccepted") || !strings.Contains(diagrams, "LoginRejected") {
		t.Fatalf("sequence diagrams did not include both auth outcomes:\n%s", diagrams)
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
	if !strings.Contains(payload, `"messages"`) || !strings.Contains(payload, `"LoginAccepted"`) {
		t.Fatalf("JSON missing messages:\n%s", payload)
	}
	if !strings.Contains(payload, `"conversations"`) {
		t.Fatalf("JSON missing conversations:\n%s", payload)
	}
}

func TestHTMLRendersStateAndTerminalPathDiagrams(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	page, err := EmitHTML(spec)
	if err != nil {
		t.Fatal(err)
	}
	required := []string{
		"<!doctype html>",
		"reservation version 2",
		"CTL Checks",
		"eventually_terminal",
		"PASS",
		"State machine",
		"Actor Protocol Projections",
		"Metrics",
		"Terminal outcome distribution",
		"Actor Inboxes",
		"Interaction Scenarios (6)",
		`<img src=`,
		"data:image/png;base64,",
		"data:image/svg+xml;base64,",
	}
	for _, want := range required {
		if !strings.Contains(page, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
}

func TestActorProjectionLabelsSendAndReceiveHandlers(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	diagram := dotActorConversation(spec.Conversations[0], "broker")
	for _, want := range []string{
		"broker protocol projection",
		"receive CreateReservation from client",
		"send HoldRequest to supplier",
		"receive HoldGranted from supplier",
		"send ReservationConfirmed to client",
	} {
		if !strings.Contains(diagram, want) {
			t.Fatalf("actor projection missing %q:\n%s", want, diagram)
		}
	}
	if strings.Contains(diagram, `reservation_id != ""`) {
		t.Fatalf("actor projection should hide default-value guard noise:\n%s", diagram)
	}
}

func TestMetricsFromQuantitativeAnnotations(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	metrics := ComputeMetrics(spec)
	if len(metrics.Conversations) != 1 {
		t.Fatalf("conversation metrics = %d", len(metrics.Conversations))
	}
	conversation := metrics.Conversations[0]
	if !conversation.HasQuantities {
		t.Fatal("expected quantitative metrics")
	}
	if len(conversation.Scenarios) != 6 {
		t.Fatalf("scenario metrics = %d, want 6", len(conversation.Scenarios))
	}
	if len(conversation.Queues) != 1 {
		t.Fatalf("inbox metrics = %d, want 1", len(conversation.Queues))
	}
	if len(conversation.Scenarios[0].Reliability) != 3 {
		t.Fatalf("scenario reliability entries = %d, want 3", len(conversation.Scenarios[0].Reliability))
	}
	if conversation.Scenarios[0].Availability < 0.985 || conversation.Scenarios[0].Availability > 0.986 {
		t.Fatalf("scenario availability = %.6f, want about 0.985", conversation.Scenarios[0].Availability)
	}
	queue := conversation.Queues[0]
	if queue.Name != "supplier" {
		t.Fatalf("inbox name = %q", queue.Name)
	}
	if queue.Capacity <= 0 || !queue.BlocksWhenFull || queue.Status != "capacity_only" {
		t.Fatalf("invalid queue metrics: %#v", queue)
	}
}

func TestEmitMetrics(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	out := EmitMetrics(spec)
	for _, want := range []string{
		"reservation_v2",
		"scenario reservation version 2 interaction path 1: Confirmed",
		"availability broker: 0.999999 parallel=[0.999 0.999]",
		"outcome Confirmed",
		"inbox supplier",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, out)
		}
	}
}

func TestReliabilityValidationRejectsUnknownActor(t *testing.T) {
	spec := &Spec{
		Participants: []string{"client"},
		Reliability:  []ReliabilitySpec{{Actor: "server", Availability: 0.99}},
		messageIndex: map[string]bool{},
	}
	err := Validate(spec)
	if err == nil || !strings.Contains(err.Error(), "unknown actor server") {
		t.Fatalf("expected unknown actor reliability error, got %v", err)
	}
}

func TestWhenThenBranchesShareOneObservedMessage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "branch.proto"), []byte(`syntax = "proto3";
package branch;
message Draw {
  string day_id = 1;
  uint32 flour_kg = 2;
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "branch.convspec"), []byte(`spec branch

import "branch.proto"

participants
  inventory
  bakery

conversation draw {
  start Waiting

  state Waiting {
    on inventory -> bakery Draw
      when msg.day_id != ""
      when msg.flour_kg != 0 then DoughMixing chance 0.88
      when msg.flour_kg == 0 then IngredientConstrained chance 0.12
  }

  state DoughMixing accept
  state IngredientConstrained accept
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := ParseFile(filepath.Join(dir, "branch.convspec"))
	if err != nil {
		t.Fatal(err)
	}
	transitions := spec.Conversations[0].States["Waiting"].Transitions
	if len(transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(transitions))
	}
	if transitions[0].MessageType != "Draw" || transitions[1].MessageType != "Draw" {
		t.Fatalf("branches should share observed message: %#v", transitions)
	}
	if transitions[0].Target != "DoughMixing" || transitions[1].Target != "IngredientConstrained" {
		t.Fatalf("branch targets = %#v", transitions)
	}
	if len(transitions[0].Guards) != 2 || len(transitions[1].Guards) != 2 {
		t.Fatalf("branch guards = %#v", transitions)
	}
	if transitions[0].Guards[0] != "msg.day_id != \"\"" || transitions[1].Guards[0] != "msg.day_id != \"\"" {
		t.Fatalf("shared guard not preserved: %#v", transitions)
	}
	if transitions[0].Chance == nil || *transitions[0].Chance != 0.88 || transitions[1].Chance == nil || *transitions[1].Chance != 0.12 {
		t.Fatalf("branch chances = %#v", transitions)
	}
}

func TestByteAccountingExampleEnumeratesActorPairBytes(t *testing.T) {
	spec, err := ParseFile("../../examples/byte_accounting.convspec")
	if err != nil {
		t.Fatal(err)
	}
	metrics := ComputeMetrics(spec)
	if len(metrics.Conversations) != 1 {
		t.Fatalf("conversation metrics = %d, want 1", len(metrics.Conversations))
	}
	conversation := metrics.Conversations[0]
	if len(conversation.Scenarios) != 7 {
		t.Fatalf("scenario metrics = %d, want 7", len(conversation.Scenarios))
	}
	orderCreated := conversation.Scenarios[0]
	if orderCreated.Outcome != "OrderCreated" {
		t.Fatalf("first outcome = %q, want OrderCreated", orderCreated.Outcome)
	}
	flows := map[string]float64{}
	for _, flow := range orderCreated.ByteFlows {
		flows[flow.From+"->"+flow.To] = flow.Bytes
	}
	for _, route := range []string{
		"user->client",
		"client->server",
		"server->database",
		"database->server",
		"server->client",
		"server->auth",
		"auth->server",
	} {
		if flows[route] <= 0 {
			t.Fatalf("%s bytes = %.0f, want positive protobuf-derived bytes in %#v", route, flows[route], orderCreated.ByteFlows)
		}
	}

	out := EmitMetrics(spec)
	for _, want := range []string{
		"web_session_v1",
		"bytes user->client:",
		"bytes client->server:",
		"bytes server->auth:",
		"bytes database->server:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, out)
		}
	}
}

func TestBakeryDayExampleEnumeratesLoafAndMoneyFlow(t *testing.T) {
	spec, err := ParseFile("../../examples/bakery_day.convspec")
	if err != nil {
		t.Fatal(err)
	}
	results := EvaluateAssertions(spec)
	if len(results) != 6 {
		t.Fatalf("assertion results = %d, want 6", len(results))
	}
	for _, result := range results {
		if result.Error != "" {
			t.Fatalf("%s had parse/eval error: %s", result.Name, result.Error)
		}
		if !result.Pass {
			t.Fatalf("%s did not pass: %#v", result.Name, result)
		}
	}

	metrics := ComputeMetrics(spec)
	if len(metrics.Conversations) != 1 {
		t.Fatalf("conversation metrics = %d, want 1", len(metrics.Conversations))
	}
	conversation := metrics.Conversations[0]
	if len(conversation.Scenarios) != 8 {
		t.Fatalf("scenario metrics = %d, want 8", len(conversation.Scenarios))
	}
	outcomes := map[string]bool{}
	for _, scenario := range conversation.Scenarios {
		outcomes[scenario.Outcome] = true
	}
	for _, want := range []string{"SoldOutClosed", "CharityClosed", "WasteClosed", "UnderproducedSoldOut"} {
		if !outcomes[want] {
			t.Fatalf("missing outcome %s in %#v", want, outcomes)
		}
	}
	if len(conversation.Queues) != 5 {
		t.Fatalf("inbox metrics = %d, want 5", len(conversation.Queues))
	}

	out := EmitMetrics(spec)
	for _, want := range []string{
		"daily_loaf_flow_v1",
		"bytes customers->storefront",
		"inbox oven_carousel",
		"outcome CharityClosed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, out)
		}
	}
}

func TestCTLAssertionsEvaluateAgainstObservableStates(t *testing.T) {
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	results := EvaluateAssertions(spec)
	if len(results) != 4 {
		t.Fatalf("assertion results = %d, want 4", len(results))
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
	spec.Conversations[0].Asserts = append(spec.Conversations[0].Asserts, Assertion{
		Name:    "can_stay_rejected",
		Formula: "can_stabilize(Rejected)",
	})
	spec.Conversations[0].Asserts = append(spec.Conversations[0].Asserts, Assertion{
		Name:    "risks_rejection",
		Formula: "risks(Rejected)",
	})
	spec.Conversations[0].Asserts = append(spec.Conversations[0].Asserts, Assertion{
		Name:    "must_eventually_terminal",
		Formula: "mustEventually(Authenticated or Rejected)",
	})
	spec.Conversations[0].Asserts = append(spec.Conversations[0].Asserts, Assertion{
		Name:    "can_permanently_rejected",
		Formula: "canPermanently(Rejected)",
	})
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
	expr, err := parseCTL("AF(sickness -> or(AF(well), AG(dead)))")
	if err != nil {
		t.Fatal(err)
	}
	got := describeCTL(expr)
	want := "must happen sickness implies (must happen well or must become dead)"
	if got != want {
		t.Fatalf("english = %q, want %q", got, want)
	}

	expr, err = parseCTL("canPermanently(dead)")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCTL(expr); got != "may happen may become dead" {
		t.Fatalf("english = %q", got)
	}

	expr, err = parseCTL("EG(dead)")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCTL(expr); got != "may become dead" {
		t.Fatalf("english = %q", got)
	}

	expr, err = parseCTL("(and not(well) dead)")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCTL(expr); got != "not well and dead" {
		t.Fatalf("english = %q", got)
	}

	expr, err = parseCTL("alive Until dead")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCTL(expr); got != "must alive until dead" {
		t.Fatalf("english = %q", got)
	}

	expr, err = parseCTL("canUntil(alive, dead)")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCTL(expr); got != "may alive until dead" {
		t.Fatalf("english = %q", got)
	}

	expr, err = parseCTL("EF(not(well) -> EU(well, virus))")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCTL(expr); got != "may happen not well implies may well until virus" {
		t.Fatalf("english = %q", got)
	}

	expr, err = parseCTL("AG(alive -> AU(alive, dead))")
	if err != nil {
		t.Fatal(err)
	}
	if got := describeCTL(expr); got != "must become alive implies must alive until dead" {
		t.Fatalf("english = %q", got)
	}
}

func TestCTLUntilEvaluatesSemiPermanentPropositions(t *testing.T) {
	conversation := Conversation{
		Name:  "life",
		Start: "Healthy",
		Order: []string{"Healthy", "Sick", "Dead"},
		States: map[string]State{
			"Healthy": {
				Name:  "Healthy",
				Emits: []string{"alive"},
				Transitions: []Transition{{
					Target: "Sick",
				}},
			},
			"Sick": {
				Name:  "Sick",
				Emits: []string{"alive", "sickness"},
				Transitions: []Transition{{
					Target: "Dead",
				}},
			},
			"Dead": {
				Name:     "Dead",
				Terminal: "accept",
				Emits:    []string{"dead"},
			},
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
	spec, err := ParseFile("../../examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	out := EmitChecks(spec)
	for _, want := range []string{
		"PASS reservation_v2.eventually_terminal",
		"english: must become submitted implies must happen (confirmed or cancelled or rejected or expired)",
		"PASS reservation_v2.no_double_outcome",
		"PASS reservation_v2.confirmation_possible",
		"PASS reservation_v2.hold_resolves",
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
