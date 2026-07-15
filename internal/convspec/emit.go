package convspec

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func EmitMermaid(spec *Spec) string {
	var parts []string
	for _, conversation := range spec.Conversations {
		parts = append(parts, emitMermaidConversation(conversation))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func emitMermaidConversation(conversation Conversation) string {
	var b strings.Builder
	fmt.Fprintln(&b, "stateDiagram-v2")
	fmt.Fprintf(&b, "  %%%% conversation %s\n", conversation.DiagramName())
	fmt.Fprintf(&b, "  [*] --> %s\n", conversation.Start)
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		if state.Terminal == "accept" {
			fmt.Fprintf(&b, "  %s --> [*]\n", state.Name)
		} else if state.Terminal == "reject" {
			fmt.Fprintf(&b, "  %s --> Reject\n", state.Name)
			fmt.Fprintln(&b, "  Reject --> [*]")
		}
		if len(state.StateIs) > 0 {
			fmt.Fprintf(&b, "  note right of %s\n", state.Name)
			for _, emission := range state.StateIs {
				fmt.Fprintf(&b, "    state_is %s\n", emission)
			}
			fmt.Fprintln(&b, "  end note")
		}
		for _, transition := range state.Transitions {
			label := transitionSummary(transition)
			if transition.Guard != "" {
				label += " [" + transition.Guard + "]"
			}
			fmt.Fprintf(&b, "  %s --> %s: %s\n", state.Name, transition.Target, label)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func EmitMermaidSequences(spec *Spec) string {
	var parts []string
	for _, conversation := range spec.Conversations {
		parts = append(parts, emitSequenceConversation(conversation))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func emitSequenceConversation(conversation Conversation) string {
	paths := enumeratePaths(conversation)
	var diagrams []string
	for i, path := range paths {
		var b strings.Builder
		fmt.Fprintln(&b, "sequenceDiagram")
		fmt.Fprintf(&b, "  %%%% conversation %s path %d\n", conversation.DiagramName(), i+1)
		if activation, ok := conversationActivation(conversation); ok {
			writeMermaidMessage(&b, activation, "")
			fmt.Fprintf(&b, "  Note over %s: %s\n", activation.Receiver, activation.Target)
		}
		for _, step := range path {
			transition := step.Transition
			guardSuffix := ""
			if transition.Guard != "" {
				guardSuffix = " [" + transition.Guard + "]"
			}
			writeMermaidMessage(&b, transition, guardSuffix)
			fmt.Fprintf(&b, "  Note over %s: %s\n", transition.Receiver, transition.Target)
		}
		terminal := conversation.Start
		if len(path) > 0 {
			terminal = path[len(path)-1].Transition.Target
		}
		if state, ok := conversation.States[terminal]; ok && state.Terminal != "" {
			fmt.Fprintf(&b, "  Note over %s: %s\n", terminal, state.Terminal)
		} else {
			fmt.Fprintf(&b, "  Note over %s: truncated\n", terminal)
		}
		diagrams = append(diagrams, strings.TrimRight(b.String(), "\n"))
	}
	return strings.Join(diagrams, "\n\n")
}

type pathStep struct {
	State      string
	Transition Transition
}

func enumeratePaths(conversation Conversation) [][]pathStep {
	if conversation.Start == "" {
		return nil
	}
	var paths [][]pathStep
	var walk func(stateName string, path []pathStep, seen map[string]bool)
	walk = func(stateName string, path []pathStep, seen map[string]bool) {
		state := conversation.States[stateName]
		if state.Terminal != "" || len(state.Transitions) == 0 {
			paths = append(paths, append([]pathStep(nil), path...))
			return
		}
		for _, transition := range state.Transitions {
			next := append(append([]pathStep(nil), path...), pathStep{State: stateName, Transition: transition})
			if transition.Target == "" || seen[transition.Target] {
				paths = append(paths, next)
				continue
			}
			nextSeen := map[string]bool{}
			for key, value := range seen {
				nextSeen[key] = value
			}
			nextSeen[transition.Target] = true
			walk(transition.Target, next, nextSeen)
		}
	}
	walk(conversation.Start, nil, map[string]bool{conversation.Start: true})
	return paths
}

func EmitDOT(spec *Spec) string {
	var parts []string
	for _, conversation := range spec.Conversations {
		parts = append(parts, dotConversation(conversation))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func dotConversation(conversation Conversation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "digraph %s {\n", dotID(conversation.DiagramName()))
	fmt.Fprintln(&b, `  rankdir="TB";`)
	writeDarkDOTDefaults(&b, conversationTitle(conversation)+" state machine")
	fmt.Fprintln(&b, `  "__start" [label="", shape="point", color="#e5e7eb"];`)
	if activation, ok := conversationActivation(conversation); ok {
		fmt.Fprintf(&b, "  \"__start\" -> %q [label=\"%s\"];\n", conversation.Start, dotEscape(transitionSummary(activation)))
	} else {
		fmt.Fprintf(&b, "  \"__start\" -> %q;\n", conversation.Start)
	}
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		fmt.Fprintf(&b, "  %q [%s];\n", state.Name, strings.Join(dotStateAttrs(state, stateLabel(state)), ", "))
	}
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		for _, transition := range state.Transitions {
			label := transitionSummary(transition)
			if transition.Guard != "" {
				label += "\nwhen " + transition.Guard
			}
			fmt.Fprintf(&b, "  %q -> %q [label=\"%s\"];\n", state.Name, transition.Target, dotEscape(label))
		}
	}
	fmt.Fprintln(&b, "}")
	return strings.TrimRight(b.String(), "\n")
}

func dotActorConversation(conversation Conversation, actor string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "digraph %s_actor_%s {\n", dotID(conversation.DiagramName()), dotID(actor))
	fmt.Fprintln(&b, `  rankdir="TB";`)
	writeDarkDOTDefaults(&b, conversationTitle(conversation)+" · "+actor+" protocol projection")
	fmt.Fprintln(&b, `  "__start" [label="", shape="point", color="#e5e7eb"];`)
	if activation, ok := conversationActivation(conversation); ok && activation.Receiver == actor {
		fmt.Fprintf(&b, "  \"__start\" -> %q [label=\"%s\"];\n", conversation.Start, dotEscape(actorTransitionLabel(activation, actor)))
	} else {
		fmt.Fprintf(&b, "  \"__start\" -> %q;\n", conversation.Start)
	}
	used := map[string]bool{conversation.Start: true}
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		if actorTouchesState(state, actor) {
			used[stateName] = true
			for _, transition := range state.Transitions {
				if transition.Receiver == actor {
					used[transition.Target] = true
				}
			}
		}
	}
	for _, stateName := range conversation.Order {
		if !used[stateName] {
			continue
		}
		state := conversation.States[stateName]
		fmt.Fprintf(&b, "  %q [%s];\n", state.Name, strings.Join(dotStateAttrs(state, stateLabel(state)), ", "))
	}
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		for _, transition := range state.Transitions {
			if transition.Receiver != actor {
				continue
			}
			label := actorTransitionLabel(transition, actor)
			if transition.Guard != "" && !isDefaultValueGuard(transition.Guard) {
				label += "\nwhen " + transition.Guard
			}
			fmt.Fprintf(&b, "  %q -> %q [label=\"%s\"];\n", state.Name, transition.Target, dotEscape(label))
		}
	}
	fmt.Fprintln(&b, "}")
	return strings.TrimRight(b.String(), "\n")
}

func actorTouchesState(state State, actor string) bool {
	for _, transition := range state.Transitions {
		if transition.Receiver == actor {
			return true
		}
	}
	return false
}

func actorTransitionLabel(transition Transition, actor string) string {
	return fmt.Sprintf("receive %s", transition.MessageType)
}

func transitionSummary(transition Transition) string {
	return fmt.Sprintf("%s receives %s", transition.Receiver, transition.MessageType)
}

func conversationActivation(conversation Conversation) (Transition, bool) {
	if conversation.StartActor == "" || conversation.StartMessage == "" {
		return Transition{}, false
	}
	return Transition{Receiver: conversation.StartActor, MessageType: conversation.StartMessage, Target: conversation.Start}, true
}

func writeMermaidMessage(b *strings.Builder, transition Transition, guardSuffix string) {
	fmt.Fprintf(b, "  %s->>%s: %s%s\n", transition.Receiver, transition.Receiver, transition.MessageType, guardSuffix)
}

func dotPath(conversation Conversation, index int, path []pathStep) string {
	var b strings.Builder
	fmt.Fprintf(&b, "digraph %s_path_%d {\n", dotID(conversation.DiagramName()), index)
	fmt.Fprintln(&b, `  rankdir="TB";`)
	fmt.Fprintf(&b, "  graph [label=\"%s\", labelloc=\"t\", fontsize=\"22\", fontname=\"Helvetica\", fontcolor=\"#e5e7eb\", bgcolor=\"#0f172a\", color=\"#334155\"];\n", dotEscape(pathTitle(conversation, index, path)))
	fmt.Fprintln(&b, `  node [shape="box", style="filled,rounded", fillcolor="#111827", color="#64748b", fontcolor="#e5e7eb", fontname="Helvetica"];`)
	fmt.Fprintln(&b, `  edge [color="#94a3b8", fontcolor="#e5e7eb", fontname="Helvetica", fontsize="12"];`)
	fmt.Fprintln(&b, `  "__start" [label="", shape="point", color="#e5e7eb"];`)
	if activation, ok := conversationActivation(conversation); ok {
		fmt.Fprintf(&b, "  \"__start\" -> %q [label=\"%s\"];\n", conversation.Start, dotEscape(transitionSummary(activation)))
	} else {
		fmt.Fprintf(&b, "  \"__start\" -> %q;\n", conversation.Start)
	}

	states := []string{conversation.Start}
	for _, step := range path {
		states = append(states, step.Transition.Target)
	}
	for i, stateName := range states {
		if stateName == "" {
			continue
		}
		state := conversation.States[stateName]
		label := state.Name
		if i == 0 {
			label += "\nstart"
		}
		fmt.Fprintf(&b, "  %q [%s];\n", stateName, strings.Join(dotStateAttrs(state, label), ", "))
	}
	for _, step := range path {
		transition := step.Transition
		label := transitionSummary(transition)
		if transition.Guard != "" {
			label += "\nwhen " + transition.Guard
		}
		fmt.Fprintf(&b, "  %q -> %q [label=\"%s\"];\n", step.State, transition.Target, dotEscape(label))
	}
	fmt.Fprintln(&b, "}")
	return strings.TrimRight(b.String(), "\n")
}

func writeDarkDOTDefaults(b *strings.Builder, title string) {
	fmt.Fprintf(b, "  graph [label=\"%s\", labelloc=\"t\", fontsize=\"22\", fontname=\"Helvetica\", fontcolor=\"#e5e7eb\", bgcolor=\"#0f172a\", color=\"#334155\"];\n", dotEscape(title))
	fmt.Fprintln(b, `  node [shape="box", style="filled,rounded", fillcolor="#111827", color="#64748b", fontcolor="#e5e7eb", fontname="Helvetica"];`)
	fmt.Fprintln(b, `  edge [color="#94a3b8", fontcolor="#e5e7eb", fontname="Helvetica", fontsize="12"];`)
}

func dotStateAttrs(state State, label string) []string {
	attrs := []string{fmt.Sprintf(`label="%s"`, dotEscape(label))}
	switch state.Terminal {
	case "accept":
		attrs = append(attrs, `shape="doublecircle"`, `fillcolor="#052e16"`, `color="#22c55e"`)
	case "reject":
		attrs = append(attrs, `shape="octagon"`, `fillcolor="#450a0a"`, `color="#ef4444"`)
	}
	return attrs
}

func pathTitle(conversation Conversation, index int, path []pathStep) string {
	terminal := conversation.Start
	if len(path) > 0 {
		terminal = path[len(path)-1].Transition.Target
	}
	return fmt.Sprintf("%s interaction path %d: %s", conversationTitle(conversation), index, terminal)
}

func stateLabel(state State) string {
	label := state.Name
	for _, emission := range state.StateIs {
		label += "\nstate_is " + emission
	}
	return label
}

func dotEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

var dotIDRE = regexp.MustCompile(`\W+`)

func dotID(value string) string {
	return dotIDRE.ReplaceAllString(value, "_")
}

func EmitJSON(spec *Spec) (string, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(spec); err != nil {
		return "", err
	}
	return b.String(), nil
}

func EmitChecks(spec *Spec) string {
	results := EvaluateAssertions(spec)
	var b strings.Builder
	if len(results) == 0 {
		fmt.Fprintln(&b, "no assertions")
		return b.String()
	}
	for _, result := range results {
		status := "FAIL"
		if result.Error != "" {
			status = "ERROR"
		} else if result.Pass {
			status = "PASS"
		}
		fmt.Fprintf(&b, "%s %s.%s: %s\n", status, result.Conversation, result.Name, result.Formula)
		if result.English != "" {
			fmt.Fprintf(&b, "  english: %s\n", result.English)
		}
		if result.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", result.Error)
		}
	}
	return b.String()
}

func EmitMetrics(spec *Spec) string {
	metrics := ComputeMetrics(spec)
	var b strings.Builder
	for _, conversation := range metrics.Conversations {
		fmt.Fprintf(&b, "%s\n", conversation.Name)
		for _, scenario := range conversation.Scenarios {
			fmt.Fprintf(&b, "  scenario %s: p=%.4f dwell=%.1fms bytes=%.0f", scenario.Name, scenario.Probability, scenario.LatencyMS, scenario.Bytes)
			if scenario.Availability > 0 {
				fmt.Fprintf(&b, " availability=%.6f", scenario.Availability)
			}
			fmt.Fprintf(&b, " outcome=%s\n", scenario.Outcome)
			for _, reliability := range scenario.Reliability {
				if len(reliability.Parallel) > 0 {
					fmt.Fprintf(&b, "    availability %s: %.6f parallel=%v\n", reliability.Actor, reliability.Availability, reliability.Parallel)
				} else {
					fmt.Fprintf(&b, "    availability %s: %.6f\n", reliability.Actor, reliability.Availability)
				}
			}
			for _, flow := range scenario.ByteFlows {
				if flow.From == "" {
					fmt.Fprintf(&b, "    bytes %s inbox: %.0f\n", flow.To, flow.Bytes)
				} else {
					fmt.Fprintf(&b, "    bytes %s->%s: %.0f\n", flow.From, flow.To, flow.Bytes)
				}
			}
		}
		for _, outcome := range conversation.Outcomes {
			fmt.Fprintf(&b, "  outcome %s: p=%.4f\n", outcome.Name, outcome.Probability)
		}
		for _, inbox := range conversation.Inboxes {
			fmt.Fprintf(&b, "  inbox %s: capacity=%d offered_load=%.3f full_probability=%.6f blocks_when_full=%t status=%s\n", inbox.Name, inbox.Capacity, inbox.OfferedLoad, inbox.FullProbability, inbox.BlocksWhenFull, inbox.Status)
		}
		for _, chart := range conversation.Charts {
			fmt.Fprintf(&b, "  chart %s: type=%s", chart.Name, chart.Chart)
			if chart.Message != "" {
				fmt.Fprintf(&b, " message=%s", chart.Message)
			}
			if chart.Value != "" {
				fmt.Fprintf(&b, " value=%s", chart.Value)
			}
			if chart.GroupBy != "" {
				fmt.Fprintf(&b, " group_by=%s", chart.GroupBy)
			}
			if chart.Window != "" {
				fmt.Fprintf(&b, " window=%s", chart.Window)
			}
			if chart.Reducer != "" {
				fmt.Fprintf(&b, " reducer=%s", chart.Reducer)
			}
			fmt.Fprintln(&b)
		}
		for _, warning := range conversation.Warnings {
			fmt.Fprintf(&b, "  warning: %s\n", warning)
		}
	}
	return b.String()
}

func EmitHTML(spec *Spec) (string, error) {
	images, err := renderReportImages(spec, "", "")
	if err != nil {
		return "", err
	}
	return htmlReport(spec, images), nil
}

func WriteHTMLReport(spec *Spec, outputPath string) error {
	if outputPath == "" {
		return fmt.Errorf("HTML output requires an output path")
	}
	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	base := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))
	assetDirName := base + "_assets"
	assetDir := filepath.Join(filepath.Dir(outputPath), assetDirName)
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		return err
	}
	images, err := renderReportImages(spec, assetDir, assetDirName)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(htmlReport(spec, images)), 0o644)
}

type reportConversation struct {
	Conversation Conversation
	StateImage   reportImage
	ActorImages  []reportImage
	PathImages   []reportImage
}

type reportImage struct {
	Title string
	Src   string
}

type RenderedConversation struct {
	Title       string          `json:"title"`
	StateImage  RenderedImage   `json:"state_image"`
	ActorImages []RenderedImage `json:"actor_images,omitempty"`
	PathImages  []RenderedImage `json:"path_images"`
}

type RenderedImage struct {
	Title string `json:"title"`
	Src   string `json:"src"`
}

func RenderImageReport(spec *Spec) ([]RenderedConversation, error) {
	reports, err := renderReportImages(spec, "", "")
	if err != nil {
		return nil, err
	}
	rendered := make([]RenderedConversation, 0, len(reports))
	for _, report := range reports {
		conversation := report.Conversation
		item := RenderedConversation{
			Title:       conversationTitle(conversation),
			StateImage:  RenderedImage(report.StateImage),
			ActorImages: make([]RenderedImage, 0, len(report.ActorImages)),
			PathImages:  make([]RenderedImage, 0, len(report.PathImages)),
		}
		for _, image := range report.ActorImages {
			item.ActorImages = append(item.ActorImages, RenderedImage(image))
		}
		for _, image := range report.PathImages {
			item.PathImages = append(item.PathImages, RenderedImage(image))
		}
		rendered = append(rendered, item)
	}
	return rendered, nil
}

func renderReportImages(spec *Spec, assetDir string, assetDirName string) ([]reportConversation, error) {
	reports := make([]reportConversation, 0, len(spec.Conversations))
	for _, conversation := range spec.Conversations {
		report := reportConversation{Conversation: conversation}

		stateName := dotID(conversation.DiagramName()) + "_state.png"
		stateSrc, err := renderDOTImageSource(dotConversation(conversation), assetDir, assetDirName, stateName)
		if err != nil {
			return nil, fmt.Errorf("render state diagram for %s: %w", conversation.DiagramName(), err)
		}
		report.StateImage = reportImage{Title: "State machine", Src: stateSrc}

		actors := conversationParticipants(conversation)
		report.ActorImages = make([]reportImage, 0, len(actors))
		for _, actor := range actors {
			actorName := fmt.Sprintf("%s_actor_%s.png", dotID(conversation.DiagramName()), dotID(actor))
			actorSrc, err := renderDOTImageSource(dotActorConversation(conversation, actor), assetDir, assetDirName, actorName)
			if err != nil {
				return nil, fmt.Errorf("render actor projection for %s.%s: %w", conversation.DiagramName(), actor, err)
			}
			report.ActorImages = append(report.ActorImages, reportImage{Title: actor + " protocol projection", Src: actorSrc})
		}

		paths := enumeratePaths(conversation)
		report.PathImages = make([]reportImage, 0, len(paths))
		for i, path := range paths {
			title := pathTitle(conversation, i+1, path)
			pathName := fmt.Sprintf("%s_path_%02d.svg", dotID(conversation.DiagramName()), i+1)
			pathSrc, err := renderInteractionImageSource(conversation, i+1, path, assetDir, assetDirName, pathName)
			if err != nil {
				return nil, fmt.Errorf("render path %d for %s: %w", i+1, conversation.DiagramName(), err)
			}
			report.PathImages = append(report.PathImages, reportImage{Title: title, Src: pathSrc})
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func htmlReport(spec *Spec, reports []reportConversation) string {
	metrics := ComputeMetrics(spec)
	var b strings.Builder
	fmt.Fprintln(&b, "<!doctype html>")
	fmt.Fprintln(&b, `<html lang="en">`)
	fmt.Fprintln(&b, "<head>")
	fmt.Fprintln(&b, `  <meta charset="utf-8">`)
	fmt.Fprintln(&b, `  <meta name="viewport" content="width=device-width, initial-scale=1">`)
	fmt.Fprintf(&b, "  <title>%s conversation diagrams</title>\n", html.EscapeString(spec.Name))
	fmt.Fprintln(&b, `  <style>`)
	fmt.Fprintln(&b, `    :root { color-scheme: dark; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }`)
	fmt.Fprintln(&b, `    body { margin: 0; background: #020617; color: #e5e7eb; }`)
	fmt.Fprintln(&b, `    header { background: #0f172a; border-bottom: 1px solid #334155; padding: 24px 32px 18px; }`)
	fmt.Fprintln(&b, `    main { max-width: 1280px; margin: 0 auto; padding: 24px 32px 48px; }`)
	fmt.Fprintln(&b, `    h1 { font-size: 24px; line-height: 1.2; margin: 0 0 6px; font-weight: 650; }`)
	fmt.Fprintln(&b, `    h2 { font-size: 18px; margin: 28px 0 12px; }`)
	fmt.Fprintln(&b, `    h3 { font-size: 15px; margin: 18px 0 8px; }`)
	fmt.Fprintln(&b, `    .meta { color: #94a3b8; font-size: 14px; }`)
	fmt.Fprintln(&b, `    .diagram, .checks { background: #0f172a; border: 1px solid #334155; border-radius: 8px; overflow: auto; padding: 18px; margin: 12px 0 22px; }`)
	fmt.Fprintln(&b, `    .paths { display: grid; gap: 18px; }`)
	fmt.Fprintln(&b, `    .paths .diagram { margin: 0; }`)
	fmt.Fprintln(&b, `    img { display: block; width: 100%; height: auto; }`)
	fmt.Fprintln(&b, `    a.image-link { display: block; }`)
	fmt.Fprintln(&b, `    code { background: #1e293b; border-radius: 4px; padding: 1px 5px; }`)
	fmt.Fprintln(&b, `    .pass { color: #86efac; font-weight: 700; }`)
	fmt.Fprintln(&b, `    .fail, .error { color: #fca5a5; font-weight: 700; }`)
	fmt.Fprintln(&b, `    .checks li { margin: 8px 0; }`)
	fmt.Fprintln(&b, `    ul { margin: 8px 0 0; padding-left: 20px; }`)
	fmt.Fprintln(&b, `  </style>`)
	fmt.Fprintln(&b, "</head>")
	fmt.Fprintln(&b, "<body>")
	fmt.Fprintln(&b, "  <header>")
	fmt.Fprintf(&b, "    <h1>%s</h1>\n", html.EscapeString(spec.Name))
	fmt.Fprintf(&b, "    <div class=\"meta\">Source <code>%s</code>", html.EscapeString(spec.SourcePath))
	if len(spec.Imports) > 0 {
		fmt.Fprintf(&b, " · Imports %s", html.EscapeString(strings.Join(spec.Imports, ", ")))
	}
	fmt.Fprintln(&b, "</div>")
	fmt.Fprintln(&b, "  </header>")
	fmt.Fprintln(&b, "  <main>")
	for _, report := range reports {
		conversation := report.Conversation
		fmt.Fprintf(&b, "    <section>\n      <h2>%s</h2>\n", html.EscapeString(conversationTitle(conversation)))
		writeAssertionChecks(&b, conversation)
		writeMetrics(&b, metricsForConversation(metrics, conversation.DiagramName()))
		fmt.Fprintln(&b, `      <div class="meta">State machine</div>`)
		fmt.Fprintln(&b, `      <div class="diagram">`)
		writeImage(&b, report.StateImage)
		fmt.Fprintln(&b, "      </div>")
		if len(report.ActorImages) > 0 {
			fmt.Fprintf(&b, "      <h3>Actor Protocol Projections (%d)</h3>\n", len(report.ActorImages))
			fmt.Fprintln(&b, `      <div class="paths">`)
			for _, image := range report.ActorImages {
				fmt.Fprintln(&b, `        <div class="diagram">`)
				fmt.Fprintf(&b, "          <div class=\"meta\">%s</div>\n", html.EscapeString(image.Title))
				writeImage(&b, image)
				fmt.Fprintln(&b, "        </div>")
			}
			fmt.Fprintln(&b, "      </div>")
		}

		fmt.Fprintf(&b, "      <h3>Interaction Scenarios (%d)</h3>\n", len(report.PathImages))
		fmt.Fprintln(&b, `      <div class="paths">`)
		for _, image := range report.PathImages {
			fmt.Fprintln(&b, `        <div class="diagram">`)
			fmt.Fprintf(&b, "          <div class=\"meta\">%s</div>\n", html.EscapeString(image.Title))
			writeImage(&b, image)
			fmt.Fprintln(&b, "        </div>")
		}
		fmt.Fprintln(&b, "      </div>")
		fmt.Fprintln(&b, "    </section>")
	}
	if len(spec.Actors) > 0 {
		fmt.Fprintln(&b, "    <section>")
		fmt.Fprintln(&b, "      <h2>Actors</h2>")
		fmt.Fprintln(&b, "      <ul>")
		for _, actor := range spec.Actors {
			fmt.Fprintf(&b, "        <li><code>%s</code>: capacity %d unread messages</li>\n", html.EscapeString(actor.Name), actor.Capacity)
		}
		fmt.Fprintln(&b, "      </ul>")
		fmt.Fprintln(&b, "    </section>")
	}
	fmt.Fprintln(&b, "  </main>")
	fmt.Fprintln(&b, "</body>")
	fmt.Fprintln(&b, "</html>")
	return b.String()
}

func metricsForConversation(report MetricsReport, name string) ConversationMetrics {
	for _, conversation := range report.Conversations {
		if conversation.Name == name {
			return conversation
		}
	}
	return ConversationMetrics{}
}

func writeMetrics(b *strings.Builder, metrics ConversationMetrics) {
	if !metrics.HasQuantities {
		return
	}
	fmt.Fprintln(b, `      <h3>Metrics</h3>`)
	fmt.Fprintln(b, `      <div class="checks">`)
	fmt.Fprintln(b, `        <div class="meta">Estimated from chance, dwell time, protobuf byte estimates, actor inbox capacities, and reliability annotations.</div>`)
	fmt.Fprintln(b, outcomeChartSVG(metrics.Outcomes))
	fmt.Fprintln(b, scenarioChartSVG(metrics.Scenarios))
	if len(metrics.Inboxes) > 0 {
		fmt.Fprintln(b, `        <h3>Actor Inboxes</h3>`)
		fmt.Fprintln(b, `        <ul>`)
		for _, inbox := range metrics.Inboxes {
			fmt.Fprintf(b, `          <li><code>%s</code>: capacity %d, FIFO consumption, writes block when full, status %s</li>`+"\n", html.EscapeString(inbox.Name), inbox.Capacity, html.EscapeString(inbox.Status))
		}
		fmt.Fprintln(b, `        </ul>`)
	}
	if len(metrics.Charts) > 0 {
		fmt.Fprintln(b, `        <h3>Declared Metric Views</h3>`)
		fmt.Fprintln(b, `        <ul>`)
		for _, chart := range metrics.Charts {
			details := []string{"type " + chart.Chart}
			if chart.Message != "" {
				details = append(details, "message "+chart.Message)
			}
			if chart.Value != "" {
				details = append(details, "value "+chart.Value)
			}
			if chart.GroupBy != "" {
				details = append(details, "grouped by "+chart.GroupBy)
			}
			if chart.Window != "" {
				details = append(details, "window "+chart.Window)
			}
			if chart.Reducer != "" {
				details = append(details, "reducer "+chart.Reducer)
			}
			fmt.Fprintf(b, `          <li><code>%s</code>: %s</li>`+"\n", html.EscapeString(chart.Name), html.EscapeString(strings.Join(details, ", ")))
		}
		fmt.Fprintln(b, `        </ul>`)
	}
	for _, warning := range metrics.Warnings {
		fmt.Fprintf(b, `        <div class="fail">%s</div>`+"\n", html.EscapeString(warning))
	}
	fmt.Fprintln(b, `      </div>`)
}

func outcomeChartSVG(outcomes []OutcomeMetric) string {
	if len(outcomes) == 0 {
		return ""
	}
	const width = 760
	height := 70 + len(outcomes)*34
	var b strings.Builder
	fmt.Fprintf(&b, `<svg width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n", width, height, width, height)
	fmt.Fprintln(&b, `<rect width="100%" height="100%" rx="8" fill="#111827"/>`)
	fmt.Fprintln(&b, `<text x="18" y="28" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="16" font-weight="700">Terminal outcome distribution</text>`)
	x := 18.0
	totalWidth := 520.0
	colors := []string{"#22c55e", "#38bdf8", "#f59e0b", "#f472b6", "#a78bfa"}
	for i, outcome := range outcomes {
		w := totalWidth * outcome.Probability
		fmt.Fprintf(&b, `<rect x="%.1f" y="44" width="%.1f" height="18" fill="%s"/>`+"\n", x, w, colors[i%len(colors)])
		x += w
	}
	for i, outcome := range outcomes {
		y := 88 + i*30
		fmt.Fprintf(&b, `<rect x="18" y="%d" width="14" height="14" fill="%s"/>`+"\n", y-12, colors[i%len(colors)])
		fmt.Fprintf(&b, `<text x="42" y="%d" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="13">%s %.1f%%</text>`+"\n", y, xmlEscape(outcome.Name), outcome.Probability*100)
	}
	fmt.Fprintln(&b, `</svg>`)
	return b.String()
}

func scenarioChartSVG(scenarios []ScenarioMetric) string {
	if len(scenarios) == 0 {
		return ""
	}
	const width = 760
	height := 76 + len(scenarios)*42
	maxLatency := 1.0
	maxBytes := 1.0
	for _, scenario := range scenarios {
		if scenario.LatencyMS > maxLatency {
			maxLatency = scenario.LatencyMS
		}
		if scenario.Bytes > maxBytes {
			maxBytes = scenario.Bytes
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n", width, height, width, height)
	fmt.Fprintln(&b, `<rect width="100%" height="100%" rx="8" fill="#111827"/>`)
	fmt.Fprintln(&b, `<text x="18" y="28" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="16" font-weight="700">Scenario dwell time and traffic</text>`)
	for i, scenario := range scenarios {
		y := 62 + i*42
		latWidth := 260 * scenario.LatencyMS / maxLatency
		byteWidth := 260 * scenario.Bytes / maxBytes
		fmt.Fprintf(&b, `<text x="18" y="%d" fill="#94a3b8" font-family="Helvetica, Arial, sans-serif" font-size="12">path %d → %s</text>`+"\n", y, i+1, xmlEscape(scenario.Outcome))
		fmt.Fprintf(&b, `<rect x="160" y="%d" width="%.1f" height="10" fill="#38bdf8"/>`+"\n", y-12, latWidth)
		fmt.Fprintf(&b, `<rect x="160" y="%d" width="%.1f" height="10" fill="#f59e0b"/>`+"\n", y+2, byteWidth)
		fmt.Fprintf(&b, `<text x="432" y="%d" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="12">%.1fms · %.0fB · p %.2f</text>`+"\n", y, scenario.LatencyMS, scenario.Bytes, scenario.Probability)
	}
	fmt.Fprintln(&b, `</svg>`)
	return b.String()
}

func writeAssertionChecks(b *strings.Builder, conversation Conversation) {
	if len(conversation.Asserts) == 0 {
		return
	}
	spec := &Spec{Conversations: []Conversation{conversation}}
	results := EvaluateAssertions(spec)
	fmt.Fprintln(b, `      <h3>CTL Checks</h3>`)
	fmt.Fprintln(b, `      <div class="checks">`)
	fmt.Fprintln(b, `        <ul>`)
	for _, result := range results {
		status := "fail"
		label := "FAIL"
		if result.Error != "" {
			status = "error"
			label = "ERROR"
		} else if result.Pass {
			status = "pass"
			label = "PASS"
		}
		fmt.Fprintf(b, "          <li><span class=\"%s\">%s</span> <code>%s</code>: %s", status, label, html.EscapeString(result.Name), html.EscapeString(result.Formula))
		if result.English != "" {
			fmt.Fprintf(b, "<br><span class=\"meta\">%s</span>", html.EscapeString(result.English))
		}
		if result.Error != "" {
			fmt.Fprintf(b, "<br><span class=\"meta\">%s</span>", html.EscapeString(result.Error))
		}
		fmt.Fprintln(b, "</li>")
	}
	fmt.Fprintln(b, `        </ul>`)
	fmt.Fprintln(b, `      </div>`)
}

func writeImage(b *strings.Builder, image reportImage) {
	escapedSrc := html.EscapeString(image.Src)
	escapedTitle := html.EscapeString(image.Title)
	if strings.HasPrefix(image.Src, "data:") {
		fmt.Fprintf(b, "        <img src=%q alt=%q>\n", escapedSrc, escapedTitle)
		return
	}
	fmt.Fprintf(b, "        <a class=\"image-link\" href=%q><img src=%q alt=%q></a>\n", escapedSrc, escapedSrc, escapedTitle)
}

func renderInteractionImageSource(conversation Conversation, index int, path []pathStep, assetDir string, assetDirName string, name string) (string, error) {
	svg := interactionSVG(conversation, index, path)
	if assetDir == "" {
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)), nil
	}
	pathName := filepath.Join(assetDir, name)
	if err := os.WriteFile(pathName, []byte(svg), 0o644); err != nil {
		return "", err
	}
	if info, err := os.Stat(pathName); err != nil {
		return "", err
	} else if info.Size() == 0 {
		return "", fmt.Errorf("interaction renderer produced empty image %s", pathName)
	}
	return filepath.ToSlash(filepath.Join(assetDirName, name)), nil
}

func interactionSVG(conversation Conversation, index int, path []pathStep) string {
	const (
		leftPad          = 280
		topPad           = 92
		participantGap   = 240
		rowGap           = 156
		participantWidth = 150
	)
	width := leftPad + max(1, len(conversationParticipants(conversation)))*participantGap + 80
	activation, hasActivation := conversationActivation(conversation)
	eventCount := len(path)
	if hasActivation {
		eventCount++
	}
	height := topPad + eventCount*rowGap + 150
	participants := conversationParticipants(conversation)
	xpos := map[string]int{}
	for i, participant := range participants {
		xpos[participant] = leftPad + i*participantGap
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", width, height, width, height)
	fmt.Fprintln(&b, `<defs>`)
	fmt.Fprintln(&b, `  <marker id="arrow" markerWidth="10" markerHeight="10" refX="8" refY="3" orient="auto" markerUnits="strokeWidth"><path d="M0,0 L0,6 L9,3 z" fill="#93c5fd"/></marker>`)
	fmt.Fprintln(&b, `</defs>`)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#0f172a"/>`+"\n")
	fmt.Fprintf(&b, `<text x="24" y="36" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="24" font-weight="700">%s</text>`+"\n", xmlEscape(pathTitle(conversation, index, path)))

	lifeTop := 62
	lifeBottom := height - 72
	for _, participant := range participants {
		x := xpos[participant]
		fmt.Fprintf(&b, `<rect x="%d" y="54" width="%d" height="38" rx="8" fill="#111827" stroke="#64748b"/>`+"\n", x-participantWidth/2, participantWidth)
		fmt.Fprintf(&b, `<text x="%d" y="78" text-anchor="middle" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="16" font-weight="700">%s</text>`+"\n", x, xmlEscape(participant))
		fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#475569" stroke-width="2" stroke-dasharray="7 7"/>`+"\n", x, lifeTop, x, lifeBottom)
	}

	row := 0
	if hasActivation {
		y := topPad + 78
		rx := xpos[activation.Receiver]
		sx := rx - 120
		mid := (sx + rx) / 2
		if rx != 0 {
			writeStateNote(&b, 24, y-54, "new conversation", activation.Target)
			fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#93c5fd" stroke-width="2.4" marker-end="url(#arrow)"/>`+"\n", sx, y, rx, y)
			for lineIndex, line := range interactionLabelLines(activation) {
				fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="14">%s</text>`+"\n", mid, y-38+lineIndex*18, xmlEscape(line))
			}
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="4" fill="#93c5fd"/>`+"\n", rx, y)
		}
		row++
	}

	for _, step := range path {
		y := topPad + 78 + row*rowGap
		transition := step.Transition
		rx := xpos[transition.Receiver]
		sx := rx - 120
		mid := (sx + rx) / 2
		if rx == 0 {
			continue
		}

		writeStateNote(&b, 24, y-54, step.State, transition.Target)
		fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#93c5fd" stroke-width="2.4" marker-end="url(#arrow)"/>`+"\n", sx, y, rx, y)
		for lineIndex, line := range interactionLabelLines(transition) {
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="14">%s</text>`+"\n", mid, y-38+lineIndex*18, xmlEscape(line))
		}
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="4" fill="#93c5fd"/>`+"\n", rx, y)
		row++
	}

	terminal := conversation.Start
	if len(path) > 0 {
		terminal = path[len(path)-1].Transition.Target
	}
	state := conversation.States[terminal]
	fmt.Fprintf(&b, `<rect x="24" y="%d" width="%d" height="46" rx="8" fill="#052e16" stroke="#22c55e"/>`+"\n", height-58, width-48)
	outcome := terminal
	if state.Terminal == "" {
		outcome = terminal + " (truncated)"
	}
	fmt.Fprintf(&b, `<text x="44" y="%d" fill="#bbf7d0" font-family="Helvetica, Arial, sans-serif" font-size="16" font-weight="700">outcome: %s</text>`+"\n", height-29, xmlEscape(outcome))
	fmt.Fprintln(&b, `</svg>`)
	return b.String()
}

func conversationParticipants(conversation Conversation) []string {
	seen := map[string]bool{}
	var participants []string
	if conversation.StartActor != "" {
		seen[conversation.StartActor] = true
		participants = append(participants, conversation.StartActor)
	}
	for _, stateName := range conversation.Order {
		for _, transition := range conversation.States[stateName].Transitions {
			if !seen[transition.Receiver] {
				seen[transition.Receiver] = true
				participants = append(participants, transition.Receiver)
			}
		}
	}
	return participants
}

func writeStateNote(b *strings.Builder, x int, y int, previous string, next string) {
	fmt.Fprintf(b, `<rect x="%d" y="%d" width="210" height="52" rx="8" fill="#1e293b" stroke="#64748b"/>`+"\n", x, y)
	fmt.Fprintf(b, `<text x="%d" y="%d" fill="#94a3b8" font-family="Helvetica, Arial, sans-serif" font-size="12">state transition</text>`+"\n", x+12, y+18)
	fmt.Fprintf(b, `<text x="%d" y="%d" fill="#e5e7eb" font-family="Helvetica, Arial, sans-serif" font-size="14" font-weight="700">%s → %s</text>`+"\n", x+12, y+39, xmlEscape(previous), xmlEscape(next))
}

func interactionLabelLines(transition Transition) []string {
	lines := []string{transition.MessageType}
	if transition.Guard != "" && !isDefaultValueGuard(transition.Guard) {
		lines = append(lines, "when "+transition.Guard)
	}
	return lines
}

func isDefaultValueGuard(guard string) bool {
	parts := strings.Fields(guard)
	if len(parts) != 3 {
		return false
	}
	if parts[1] != "==" && parts[1] != "!=" {
		return false
	}
	switch parts[2] {
	case `""`, "0", "0.0", "false":
		return true
	default:
		return false
	}
}

func xmlEscape(value string) string {
	return html.EscapeString(value)
}

func conversationTitle(conversation Conversation) string {
	if conversation.Version == "" {
		return conversation.Name
	}
	return conversation.Name + " version " + conversation.Version
}

func sequenceDiagramForPath(conversation Conversation, index int, path []pathStep) string {
	var b strings.Builder
	fmt.Fprintln(&b, "sequenceDiagram")
	fmt.Fprintf(&b, "  %%%% conversation %s path %d\n", conversation.DiagramName(), index)
	if activation, ok := conversationActivation(conversation); ok {
		writeMermaidMessage(&b, activation, "")
		fmt.Fprintf(&b, "  Note over %s: %s\n", activation.Receiver, activation.Target)
	}
	for _, step := range path {
		transition := step.Transition
		guardSuffix := ""
		if transition.Guard != "" {
			guardSuffix = " [" + transition.Guard + "]"
		}
		writeMermaidMessage(&b, transition, guardSuffix)
		fmt.Fprintf(&b, "  Note over %s: %s\n", transition.Receiver, transition.Target)
	}
	terminal := conversation.Start
	if len(path) > 0 {
		terminal = path[len(path)-1].Transition.Target
	}
	if state, ok := conversation.States[terminal]; ok && state.Terminal != "" {
		fmt.Fprintf(&b, "  Note over %s: %s\n", terminal, state.Terminal)
	} else {
		fmt.Fprintf(&b, "  Note over %s: truncated\n", terminal)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderDOTImageSource(dotSource string, assetDir string, assetDirName string, name string) (string, error) {
	image, err := renderDOTPNG(dotSource)
	if err != nil {
		return "", err
	}
	if assetDir == "" {
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(image), nil
	}
	path := filepath.Join(assetDir, name)
	if err := os.WriteFile(path, image, 0o644); err != nil {
		return "", err
	}
	if info, err := os.Stat(path); err != nil {
		return "", err
	} else if info.Size() == 0 {
		return "", fmt.Errorf("dot produced empty image %s", path)
	}
	return filepath.ToSlash(filepath.Join(assetDirName, name)), nil
}

func renderDOTPNG(dotSource string) ([]byte, error) {
	cmd := exec.Command("dot", "-Tpng")
	cmd.Stdin = strings.NewReader(dotSource)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("dot -Tpng failed: %s", detail)
	}
	if len(output) == 0 {
		return nil, fmt.Errorf("dot produced empty PNG")
	}
	return output, nil
}
