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
		if len(state.Emits) > 0 {
			fmt.Fprintf(&b, "  note right of %s\n", state.Name)
			for _, emission := range state.Emits {
				fmt.Fprintf(&b, "    emits %s\n", emission)
			}
			fmt.Fprintln(&b, "  end note")
		}
		for _, transition := range state.Transitions {
			label := fmt.Sprintf("%s->%s: %s", transition.Sender, transition.Receiver, transition.MessageType)
			if len(transition.Guards) > 0 {
				label += " [" + strings.Join(transition.Guards, "; ") + "]"
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
		for _, step := range path {
			transition := step.Transition
			guardSuffix := ""
			if len(transition.Guards) > 0 {
				guardSuffix = " [" + strings.Join(transition.Guards, "; ") + "]"
			}
			fmt.Fprintf(&b, "  %s->>%s: %s%s\n", transition.Sender, transition.Receiver, transition.MessageType, guardSuffix)
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
	fmt.Fprintf(&b, "  \"__start\" -> %q;\n", conversation.Start)
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		fmt.Fprintf(&b, "  %q [%s];\n", state.Name, strings.Join(dotStateAttrs(state, stateLabel(state)), ", "))
	}
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		for _, transition := range state.Transitions {
			label := fmt.Sprintf("%s->%s: %s", transition.Sender, transition.Receiver, transition.MessageType)
			if transition.Bind != "" {
				label += "\nbind " + transition.Bind
			}
			for _, guard := range transition.Guards {
				label += "\nwhen " + guard
			}
			fmt.Fprintf(&b, "  %q -> %q [label=\"%s\"];\n", state.Name, transition.Target, dotEscape(label))
		}
	}
	fmt.Fprintln(&b, "}")
	return strings.TrimRight(b.String(), "\n")
}

func dotPath(conversation Conversation, index int, path []pathStep) string {
	var b strings.Builder
	fmt.Fprintf(&b, "digraph %s_path_%d {\n", dotID(conversation.DiagramName()), index)
	fmt.Fprintln(&b, `  rankdir="TB";`)
	fmt.Fprintf(&b, "  graph [label=\"%s\", labelloc=\"t\", fontsize=\"22\", fontname=\"Helvetica\", fontcolor=\"#e5e7eb\", bgcolor=\"#0f172a\", color=\"#334155\"];\n", dotEscape(pathTitle(conversation, index, path)))
	fmt.Fprintln(&b, `  node [shape="box", style="filled,rounded", fillcolor="#111827", color="#64748b", fontcolor="#e5e7eb", fontname="Helvetica"];`)
	fmt.Fprintln(&b, `  edge [color="#94a3b8", fontcolor="#e5e7eb", fontname="Helvetica", fontsize="12"];`)
	fmt.Fprintln(&b, `  "__start" [label="", shape="point", color="#e5e7eb"];`)
	fmt.Fprintf(&b, "  \"__start\" -> %q;\n", conversation.Start)

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
		if state.Terminal != "" {
			label += "\n" + state.Terminal
		}
		fmt.Fprintf(&b, "  %q [%s];\n", stateName, strings.Join(dotStateAttrs(state, label), ", "))
	}
	for _, step := range path {
		transition := step.Transition
		label := fmt.Sprintf("%s->%s: %s", transition.Sender, transition.Receiver, transition.MessageType)
		if transition.Bind != "" {
			label += "\nbind " + transition.Bind
		}
		for _, guard := range transition.Guards {
			label += "\nwhen " + guard
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
	for _, emission := range state.Emits {
		label += "\nemits " + emission
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
		if result.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", result.Error)
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
	PathImages   []reportImage
}

type reportImage struct {
	Title string
	Src   string
}

type RenderedConversation struct {
	Title      string          `json:"title"`
	StateImage RenderedImage   `json:"state_image"`
	PathImages []RenderedImage `json:"path_images"`
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
			Title:      conversationTitle(conversation),
			StateImage: RenderedImage(report.StateImage),
			PathImages: make([]RenderedImage, 0, len(report.PathImages)),
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

		paths := enumeratePaths(conversation)
		report.PathImages = make([]reportImage, 0, len(paths))
		for i, path := range paths {
			pathName := fmt.Sprintf("%s_path_%02d.png", dotID(conversation.DiagramName()), i+1)
			pathSrc, err := renderDOTImageSource(dotPath(conversation, i+1, path), assetDir, assetDirName, pathName)
			if err != nil {
				return nil, fmt.Errorf("render path %d for %s: %w", i+1, conversation.DiagramName(), err)
			}
			report.PathImages = append(report.PathImages, reportImage{Title: pathTitle(conversation, i+1, path), Src: pathSrc})
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func htmlReport(spec *Spec, reports []reportConversation) string {
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
		fmt.Fprintln(&b, `      <div class="meta">State machine</div>`)
		fmt.Fprintln(&b, `      <div class="diagram">`)
		writeImage(&b, report.StateImage)
		fmt.Fprintln(&b, "      </div>")

		fmt.Fprintf(&b, "      <h3>Terminal Paths (%d)</h3>\n", len(report.PathImages))
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
	if len(spec.Participants) > 0 {
		fmt.Fprintln(&b, "    <section>")
		fmt.Fprintln(&b, "      <h2>Participants</h2>")
		fmt.Fprintln(&b, "      <ul>")
		for _, participant := range spec.Participants {
			fmt.Fprintf(&b, "        <li><code>%s</code></li>\n", html.EscapeString(participant))
		}
		fmt.Fprintln(&b, "      </ul>")
		fmt.Fprintln(&b, "    </section>")
	}
	fmt.Fprintln(&b, "  </main>")
	fmt.Fprintln(&b, "</body>")
	fmt.Fprintln(&b, "</html>")
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
	for _, step := range path {
		transition := step.Transition
		guardSuffix := ""
		if len(transition.Guards) > 0 {
			guardSuffix = " [" + strings.Join(transition.Guards, "; ") + "]"
		}
		fmt.Fprintf(&b, "  %s->>%s: %s%s\n", transition.Sender, transition.Receiver, transition.MessageType, guardSuffix)
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
