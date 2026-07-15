package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rfielding/spec/internal/convspec"
)

func main() {
	addr := getenv("ADDR", "127.0.0.1:18080")
	server := newServer(".")
	fmt.Fprintf(os.Stderr, "specweb listening on http://%s\n", addr)
	if err := http.ListenAndServe(addr, server.routes()); err != nil {
		fmt.Fprintf(os.Stderr, "specweb: %v\n", err)
		os.Exit(1)
	}
}

func getenv(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

type server struct {
	root string
}

func newServer(root string) *server {
	return &server{root: root}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/save", s.handleSave)
	return mux
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type sessionResponse struct {
	SpecPath          string            `json:"spec_path"`
	Files             map[string]string `json:"files"`
	OpenAIConfigured  bool              `json:"openai_configured"`
	DefaultModel      string            `json:"default_model"`
	AvailableExamples []exampleSpec     `json:"available_examples"`
}

type exampleSpec struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	specPath := r.URL.Query().Get("spec")
	if specPath == "" {
		specPath = "examples/reservation.convspec"
	}
	files, err := s.readSessionFiles(specPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{
		SpecPath:          specPath,
		Files:             files,
		OpenAIConfigured:  os.Getenv("OPENAI_API_KEY") != "",
		DefaultModel:      defaultModel(),
		AvailableExamples: availableExamples(),
	})
}

func (s *server) readSessionFiles(specPath string) (map[string]string, error) {
	specPath, err := safeRel(specPath)
	if err != nil {
		return nil, err
	}
	fullSpecPath := filepath.Join(s.root, specPath)
	data, err := os.ReadFile(fullSpecPath)
	if err != nil {
		return nil, err
	}
	files := map[string]string{specPath: string(data)}
	for _, importPath := range importedProtoPaths(string(data)) {
		rel := filepath.ToSlash(filepath.Join(filepath.Dir(specPath), importPath))
		rel, err = safeRel(rel)
		if err != nil {
			return nil, err
		}
		protoData, err := os.ReadFile(filepath.Join(s.root, rel))
		if err != nil {
			return nil, err
		}
		files[rel] = string(protoData)
	}
	return files, nil
}

var importRE = regexp.MustCompile(`(?m)^\s*import\s+"([^"]+)"`)

func importedProtoPaths(text string) []string {
	var paths []string
	for _, match := range importRE.FindAllStringSubmatch(text, -1) {
		paths = append(paths, match[1])
	}
	return paths
}

type chatRequest struct {
	Message  string            `json:"message"`
	Model    string            `json:"model"`
	SpecPath string            `json:"spec_path"`
	Files    map[string]string `json:"files"`
}

type chatResponse struct {
	Blocks   []messageBlock `json:"blocks"`
	Evidence []messageBlock `json:"evidence,omitempty"`
	Model    string         `json:"model,omitempty"`
	UsedLLM  bool           `json:"used_llm,omitempty"`
}

type messageBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Title string `json:"title,omitempty"`
	Src   string `json:"src,omitempty"`
}

type saveRequest struct {
	Files map[string]string `json:"files"`
}

type saveResponse struct {
	Saved []string `json:"saved"`
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request chatRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	response, err := s.respond(r.Context(), request)
	if err != nil {
		writeJSON(w, http.StatusOK, chatResponse{Blocks: []messageBlock{{Type: "text", Text: "Compilation failed:\n" + err.Error()}}})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request saveRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(request.Files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no files supplied"})
		return
	}
	saved := make([]string, 0, len(request.Files))
	for rel, content := range request.Files {
		rel, err := safeRel(rel)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		fullPath := filepath.Join(s.root, rel)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		saved = append(saved, rel)
	}
	writeJSON(w, http.StatusOK, saveResponse{Saved: saved})
}

func (s *server) respond(ctx context.Context, request chatRequest) (chatResponse, error) {
	specPath, err := safeRel(request.SpecPath)
	if err != nil {
		return chatResponse{}, err
	}
	if len(request.Files) == 0 {
		return chatResponse{}, errors.New("no files supplied")
	}
	tempDir, err := os.MkdirTemp("", "specweb-*")
	if err != nil {
		return chatResponse{}, err
	}
	defer os.RemoveAll(tempDir)

	for rel, content := range request.Files {
		rel, err = safeRel(rel)
		if err != nil {
			return chatResponse{}, err
		}
		fullPath := filepath.Join(tempDir, rel)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return chatResponse{}, err
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return chatResponse{}, err
		}
	}

	spec, err := convspec.ParseFile(filepath.Join(tempDir, specPath))
	if err != nil {
		return chatResponse{}, err
	}
	analysis := convspec.Analyze(spec)
	images, err := convspec.RenderImageReport(spec)
	if err != nil {
		return chatResponse{}, err
	}

	summary := compileSummary(spec, analysis)
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = defaultModel()
	}

	blocks := []messageBlock{{Type: "text", Text: summary}}
	evidence := evidenceBlocks(images)
	if smart := smartResponse(request.Message, spec, analysis); smart != "" {
		blocks = append(blocks, messageBlock{Type: "text", Text: smart})
	}
	usedLLM := false
	for _, conversation := range images {
		blocks = append(blocks, messageBlock{Type: "text", Text: fmt.Sprintf("Rendered `%s`: 1 state machine, %d actor protocol projections, %d interaction scenarios. Open the evidence panel to inspect the deterministic diagrams.", conversation.Title, len(conversation.ActorImages), len(conversation.PathImages))})
	}
	if strings.TrimSpace(request.Message) != "" {
		llmText, err := s.callOpenAI(ctx, model, buildLLMPrompt(request, specPath, summary, evidence))
		if err != nil {
			blocks = append(blocks, messageBlock{Type: "text", Text: "OpenAI response unavailable: " + err.Error() + "\n\nThe deterministic compiler evidence is still current."})
		} else {
			usedLLM = true
			blocks = append(blocks, messageBlock{Type: "text", Text: llmText})
		}
	}
	return chatResponse{Blocks: blocks, Evidence: evidence, Model: model, UsedLLM: usedLLM}, nil
}

func availableExamples() []exampleSpec {
	return []exampleSpec{
		{Path: "examples/reservation.convspec", Title: "reservation"},
		{Path: "examples/auth.convspec", Title: "authentication"},
		{Path: "examples/byte_accounting.convspec", Title: "byte accounting"},
		{Path: "examples/bakery_day.convspec", Title: "bakery day"},
		{Path: "examples/project_tooling.convspec", Title: "project tooling"},
	}
}

func defaultModel() string {
	return getenv("OPENAI_MODEL", "gpt-5.3-mini")
}

func evidenceBlocks(images []convspec.RenderedConversation) []messageBlock {
	var blocks []messageBlock
	for _, conversation := range images {
		blocks = append(blocks, messageBlock{
			Type:  "image",
			Title: conversation.Title + " state machine",
			Src:   conversation.StateImage.Src,
		})
		for _, image := range conversation.ActorImages {
			blocks = append(blocks, messageBlock{
				Type:  "image",
				Title: conversation.Title + " " + image.Title,
				Src:   image.Src,
			})
		}
		for _, image := range conversation.PathImages {
			blocks = append(blocks, messageBlock{
				Type:  "image",
				Title: conversation.Title + " " + image.Title,
				Src:   image.Src,
			})
		}
	}
	return blocks
}

func smartResponse(message string, spec *convspec.Spec, analysis convspec.AnalysisReport) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "tla"):
		return renderTLAPlusSketch(spec)
	case strings.Contains(lower, "auth") || strings.Contains(lower, "authentication"):
		return "The authentication sequence should be inspected from the interaction scenarios and actor projections. Select the authentication example when you want only login traffic in the evidence panel."
	case strings.Contains(lower, "canvas") || strings.Contains(lower, "animation"):
		return "Canvas animation is the next deterministic renderer to add. The current compiled evidence already exposes the scenario paths, queue annotations, byte counts, and probabilities that would drive a reproducible day-of-work animation."
	case strings.Contains(lower, "make today") || strings.Contains(lower, "spend") || strings.Contains(lower, "ingredients"):
		return "The money and ingredient questions need observable inputs in the spec: bakery manifest records, terminal sales records, ingredient price observations, and payroll/truck cost records. The bakery example now has the message vocabulary and queue metrics to host those records."
	case len(analysis.Assertions) > 0 && strings.Contains(lower, "temporal"):
		return "Temporal checks are evaluated by the Go compiler before the LLM responds. Treat the CTL results in the compile summary as the source of truth."
	default:
		return ""
	}
}

func renderTLAPlusSketch(spec *convspec.Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "TLA+ sketch for `%s`:\n\n", spec.Name)
	fmt.Fprintln(&b, "```tla")
	fmt.Fprintf(&b, "---- MODULE %s ----\n", sanitizeTLAName(spec.Name))
	fmt.Fprintln(&b, "EXTENDS Naturals, Sequences")
	fmt.Fprintln(&b, "VARIABLE state")
	fmt.Fprintln(&b)
	for _, conversation := range spec.Conversations {
		fmt.Fprintf(&b, "%sStates == {%s}\n", sanitizeTLAName(conversation.DiagramName()), quoteStateSet(conversation.Order))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Init == state = <<>>")
	fmt.Fprintln(&b, "Next ==")
	for i, conversation := range spec.Conversations {
		prefix := "  \\/"
		if i == 0 {
			prefix = "    "
		}
		fmt.Fprintf(&b, "%s \\* %s transition relation derived from convspec messages\n", prefix, conversation.DiagramName())
	}
	fmt.Fprintln(&b, "====")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b, "\nThis is deliberately a sketch: the deterministic compiler still owns reachability, CTL checks, byte accounting, and diagrams.")
	return b.String()
}

func sanitizeTLAName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Spec"
	}
	return b.String()
}

func quoteStateSet(states []string) string {
	if len(states) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(states))
	for _, state := range states {
		quoted = append(quoted, fmt.Sprintf("%q", state))
	}
	return strings.Join(quoted, ", ")
}

func buildLLMPrompt(request chatRequest, specPath string, summary string, evidence []messageBlock) string {
	var b strings.Builder
	fmt.Fprintln(&b, "You are helping design an externally observable protocol specification.")
	fmt.Fprintln(&b, "The Go compiler has already parsed the convspec/protobuf files and rendered deterministic evidence.")
	fmt.Fprintln(&b, "Do not invent diagram contents; refer to the compiled evidence and suggest spec edits or deterministic renderer additions.")
	fmt.Fprintln(&b, "When proposing file changes, prefer a concise explanation plus complete replacement snippets or unified diffs that a browser UI can apply in a later tool step.")
	fmt.Fprintln(&b, "Prefer literate engineering responses: state the design argument, cite the executable evidence, then suggest the smallest spec/code change needed.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "User request:\n%s\n\n", strings.TrimSpace(request.Message))
	fmt.Fprintf(&b, "Spec path: %s\n\n", specPath)
	fmt.Fprintf(&b, "Compiler summary:\n%s\n", summary)
	fmt.Fprintln(&b, "Available evidence:")
	for _, block := range evidence {
		fmt.Fprintf(&b, "- %s\n", block.Title)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Editable files:")
	total := 0
	for path, content := range request.Files {
		if total > 60000 {
			fmt.Fprintln(&b, "\n[remaining files omitted from prompt budget]")
			break
		}
		clipped := clipString(content, 18000)
		total += len(clipped)
		fmt.Fprintf(&b, "\n--- %s ---\n%s\n", path, clipped)
	}
	return b.String()
}

func clipString(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "\n[truncated]"
}

func (s *server) callOpenAI(ctx context.Context, model string, prompt string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", errors.New("OPENAI_API_KEY is not set")
	}
	body, err := json.Marshal(map[string]string{
		"model": model,
		"input": prompt,
	})
	if err != nil {
		return "", err
	}
	baseURL := strings.TrimRight(getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OpenAI API returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	text, err := extractOpenAIText(data)
	if err != nil {
		return "", err
	}
	return text, nil
}

func extractOpenAIText(data []byte) (string, error) {
	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", err
	}
	if response.Error != nil && response.Error.Message != "" {
		return "", errors.New(response.Error.Message)
	}
	if strings.TrimSpace(response.OutputText) != "" {
		return strings.TrimSpace(response.OutputText), nil
	}
	var parts []string
	for _, output := range response.Output {
		for _, content := range output.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, strings.TrimSpace(content.Text))
			}
		}
	}
	if len(parts) == 0 {
		return "", errors.New("OpenAI response did not include output text")
	}
	return strings.Join(parts, "\n\n"), nil
}

func compileSummary(spec *convspec.Spec, analysis convspec.AnalysisReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Compiled `%s` successfully.\n\n", spec.Name)
	for _, conversation := range analysis.Conversations {
		fmt.Fprintf(&b, "- `%s`: %d states, %d reachable, %d transitions, %d terminal paths\n",
			conversation.Name,
			conversation.StateCount,
			conversation.ReachableStates,
			conversation.TransitionCount,
			conversation.TerminalPathCount,
		)
		if len(conversation.AcceptStates) > 0 {
			fmt.Fprintf(&b, "  accept: %s\n", strings.Join(conversation.AcceptStates, ", "))
		}
		if len(conversation.RejectStates) > 0 {
			fmt.Fprintf(&b, "  reject: %s\n", strings.Join(conversation.RejectStates, ", "))
		}
	}
	if len(analysis.Warnings) > 0 {
		fmt.Fprintln(&b, "\nWarnings:")
		for _, warning := range analysis.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	if len(analysis.Assertions) > 0 {
		fmt.Fprintln(&b, "\nCTL checks:")
		for _, assertion := range analysis.Assertions {
			status := "FAIL"
			if assertion.Pass {
				status = "PASS"
			}
			if assertion.Error != "" {
				status = "ERROR"
			}
			fmt.Fprintf(&b, "- %s `%s`: %s\n", status, assertion.Name, assertion.Formula)
			if assertion.English != "" {
				fmt.Fprintf(&b, "  english: %s\n", assertion.English)
			}
			if assertion.Error != "" {
				fmt.Fprintf(&b, "  error: %s\n", assertion.Error)
			}
		}
	}
	metrics := convspec.ComputeMetrics(spec)
	if len(metrics.Conversations) > 0 {
		fmt.Fprintln(&b, "\nMetrics:")
		for _, conversation := range metrics.Conversations {
			if !conversation.HasQuantities {
				continue
			}
			for _, outcome := range conversation.Outcomes {
				fmt.Fprintf(&b, "- outcome `%s`: %.1f%%\n", outcome.Name, outcome.Probability*100)
			}
			for _, scenario := range conversation.Scenarios {
				if scenario.Availability > 0 {
					fmt.Fprintf(&b, "- scenario `%s`: actor availability %.4f%%\n", scenario.Name, scenario.Availability*100)
				}
			}
			for _, queue := range conversation.Queues {
				fmt.Fprintf(&b, "- queue `%s`: capacity %d, offered load %.3f, full probability %.4f%%, blocks when full, status %s\n", queue.Name, queue.Capacity, queue.OfferedLoad, queue.FullProbability*100, queue.Status)
			}
		}
	}
	return b.String()
}

func decodeJSON(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func safeRel(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("unsafe path %q", path)
	}
	return filepath.ToSlash(cleaned), nil
}

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Convspec Design Workbench</title>
  <style>
    :root { font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #e6edf3; background: #0d1117; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; overflow: hidden; }
    .shell { display: grid; grid-template-columns: minmax(320px, 31vw) minmax(380px, 1fr) minmax(300px, 28vw); height: 100vh; }
    .editor, .evidence { background: #111820; overflow: auto; padding: 16px; }
    .editor { border-right: 1px solid #263241; }
    .evidence { border-left: 1px solid #263241; }
    .chat { display: grid; grid-template-rows: auto 1fr auto; min-width: 0; background: #0d1117; }
    header { background: #111820; border-bottom: 1px solid #263241; padding: 14px 18px; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
    h1 { font-size: 17px; margin: 0; }
    h2 { font-size: 13px; margin: 18px 0 8px; color: #c9d7e3; }
    label { display: block; font-size: 12px; font-weight: 650; color: #9fb0c0; margin: 12px 0 6px; }
    select, textarea, input { width: 100%; border: 1px solid #334155; border-radius: 6px; background: #0d1117; color: #e6edf3; }
    select, input { height: 36px; padding: 0 9px; }
    textarea { min-height: 230px; resize: vertical; padding: 10px; font: 12px/1.45 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
    .status { color: #8ea2b4; font-size: 12px; white-space: nowrap; }
    .status.ready { color: #87d39c; }
    .status.local { color: #f3c677; }
    .top-actions { display: flex; align-items: center; gap: 8px; }
    .secondary { height: 34px; border: 1px solid #334155; background: #151b23; color: #c9d7e3; padding: 0 10px; }
    .messages { padding: 22px; overflow: auto; }
    .message { max-width: 880px; margin: 0 auto 18px; display: grid; grid-template-columns: 40px 1fr; gap: 12px; }
    .avatar { width: 32px; height: 32px; border-radius: 50%; display: grid; place-items: center; font-weight: 700; font-size: 11px; background: #263241; color: #dbe7f2; }
    .user .avatar { background: #2563eb; }
    .bubble { background: #151b23; border: 1px solid #263241; border-radius: 8px; padding: 14px; overflow: hidden; }
    .user .bubble { background: #122448; border-color: #1f4d96; }
    .bubble pre { white-space: pre-wrap; margin: 0; font: 14px/1.55 inherit; }
    .bubble code { color: #9be9a8; }
    .bubble img, .evidence img { display: block; width: 100%; height: auto; border: 1px solid #334155; border-radius: 6px; margin-top: 8px; background: #0b0f14; }
    .image-title { margin-top: 14px; font-size: 12px; color: #9fb0c0; font-weight: 650; }
    .composer { border-top: 1px solid #263241; background: #111820; padding: 12px 18px; display: grid; gap: 10px; }
    .chips { display: flex; flex-wrap: wrap; gap: 8px; }
    .chip { height: 30px; border: 1px solid #334155; background: #151b23; color: #c9d7e3; padding: 0 10px; font-size: 12px; }
    .composer-row { display: grid; grid-template-columns: 1fr auto; gap: 10px; }
    .composer textarea { min-height: 56px; max-height: 150px; }
    button { height: 40px; border: 0; border-radius: 6px; padding: 0 16px; background: #2f81f7; color: #fff; font-weight: 650; cursor: pointer; }
    button:disabled { opacity: .55; cursor: default; }
    .hint { color: #8ea2b4; font-size: 13px; line-height: 1.4; }
    .file-tabs { display: grid; gap: 8px; }
    .evidence-empty { color: #8ea2b4; font-size: 13px; line-height: 1.45; }
    .evidence-item { margin-bottom: 16px; }
    .evidence-title { font-size: 12px; color: #c9d7e3; font-weight: 700; margin-bottom: 6px; }
    @media (max-width: 1100px) {
      body { overflow: auto; }
      .shell { grid-template-columns: 1fr; height: auto; min-height: 100vh; }
      .editor, .evidence { border: 0; border-bottom: 1px solid #263241; max-height: none; }
      .chat { min-height: 70vh; }
    }
  </style>
</head>
<body>
  <div class="shell">
    <aside class="editor">
      <h1>Convspec Workbench</h1>
      <p class="hint">Edit the spec/proto text, then argue through the protocol with the assistant. Compiler artifacts are generated deterministically by Go.</p>
      <label for="example">Example</label>
      <select id="example">
        <option value="examples/reservation.convspec">reservation</option>
        <option value="examples/auth.convspec">authentication</option>
        <option value="examples/byte_accounting.convspec">byte accounting</option>
        <option value="examples/bakery_day.convspec">bakery day</option>
        <option value="examples/project_tooling.convspec">project tooling</option>
      </select>
      <label for="model">Model</label>
      <select id="model">
        <option value="gpt-5.3-mini">gpt-5.3-mini</option>
        <option value="gpt-5.3">gpt-5.3</option>
        <option value="gpt-5-mini">gpt-5-mini</option>
        <option value="gpt-4.1">gpt-4.1</option>
      </select>
      <label for="thread">Conversation</label>
      <select id="thread"></select>
      <button id="newThread" class="secondary" type="button">New Conversation</button>
      <div id="files"></div>
    </aside>
    <section class="chat">
      <header>
        <h1>Design Discussion</h1>
        <div class="top-actions">
          <button id="saveFiles" class="secondary" type="button">Save Files</button>
          <div id="apiStatus" class="status">checking API key</div>
        </div>
      </header>
      <main id="messages" class="messages"></main>
      <form id="composer" class="composer">
        <div class="chips">
          <button class="chip" type="button" data-prompt="Compile this and show me the current evidence.">compile evidence</button>
          <button class="chip" type="button" data-prompt="Show me the authentication sequence.">authentication sequence</button>
          <button class="chip" type="button" data-prompt="Render this to TLA+.">render TLA+</button>
          <button class="chip" type="button" data-prompt="Let's see a canvas animation of a day of work.">canvas day</button>
          <button class="chip" type="button" data-prompt="How much did we make today, and how much did we spend?">daily money</button>
          <button class="chip" type="button" data-prompt="What is the ingredients price difference vs last week?">ingredient delta</button>
          <button class="chip" type="button" data-prompt="Render this as a literate design note with diagrams, proof obligations, and implementation surfaces.">literate note</button>
        </div>
        <div class="composer-row">
          <textarea id="prompt" placeholder="Ask about scenarios, queues, money flow, byte accounting, CTL checks, or protocol compatibility."></textarea>
          <button id="send" type="submit">Send</button>
        </div>
      </form>
    </section>
    <aside class="evidence">
      <h1>Spec Evidence</h1>
      <p class="hint">State machines, actor projections, and scenario interaction diagrams appear here after each compile.</p>
      <div id="evidence"><p class="evidence-empty">No compiled evidence yet.</p></div>
    </aside>
  </div>
  <script>
    const example = document.querySelector("#example");
    const model = document.querySelector("#model");
    const thread = document.querySelector("#thread");
    const newThread = document.querySelector("#newThread");
    const saveFiles = document.querySelector("#saveFiles");
    const filesEl = document.querySelector("#files");
    const messages = document.querySelector("#messages");
    const evidence = document.querySelector("#evidence");
    const composer = document.querySelector("#composer");
    const prompt = document.querySelector("#prompt");
    const send = document.querySelector("#send");
    const apiStatus = document.querySelector("#apiStatus");
    let specPath = example.value;
    let threadID = "";
    let transcript = [];

    function loadThreads() {
      const stored = JSON.parse(localStorage.getItem("specweb.threads") || "[]");
      if (stored.length === 0) {
        const id = "thread-" + Date.now();
        stored.push({id, title: "Design thread", messages: []});
        localStorage.setItem("specweb.threads", JSON.stringify(stored));
      }
      thread.textContent = "";
      for (const item of stored) {
        const option = document.createElement("option");
        option.value = item.id;
        option.textContent = item.title;
        thread.appendChild(option);
      }
      threadID = stored[0].id;
      thread.value = threadID;
      transcript = stored[0].messages || [];
      redrawTranscript();
    }

    function saveTranscript() {
      const stored = JSON.parse(localStorage.getItem("specweb.threads") || "[]");
      const index = stored.findIndex(item => item.id === threadID);
      if (index >= 0) {
        stored[index].messages = transcript;
        if (transcript.length > 0) {
          stored[index].title = transcript[0].blocks[0]?.text?.slice(0, 42) || stored[index].title;
        }
      }
      localStorage.setItem("specweb.threads", JSON.stringify(stored));
      loadThreadOptionsOnly(stored);
    }

    function loadThreadOptionsOnly(stored) {
      const selected = threadID;
      thread.textContent = "";
      for (const item of stored) {
        const option = document.createElement("option");
        option.value = item.id;
        option.textContent = item.title;
        thread.appendChild(option);
      }
      thread.value = selected;
    }

    function redrawTranscript() {
      messages.textContent = "";
      for (const message of transcript) {
        appendMessageElement(message.role, message.blocks);
      }
    }

    function addMessage(role, blocks) {
      transcript.push({role, blocks});
      saveTranscript();
      appendMessageElement(role, blocks);
    }

    function appendMessageElement(role, blocks) {
      const wrap = document.createElement("div");
      wrap.className = "message " + role;
      const avatar = document.createElement("div");
      avatar.className = "avatar";
      avatar.textContent = role === "user" ? "You" : "AI";
      const bubble = document.createElement("div");
      bubble.className = "bubble";
      for (const block of blocks) {
        if (block.type === "text") {
          const pre = document.createElement("pre");
          pre.textContent = block.text;
          bubble.appendChild(pre);
        } else if (block.type === "image") {
          const title = document.createElement("div");
          title.className = "image-title";
          title.textContent = block.title;
          const img = document.createElement("img");
          img.src = block.src;
          img.alt = block.title;
          bubble.appendChild(title);
          bubble.appendChild(img);
        }
      }
      wrap.appendChild(avatar);
      wrap.appendChild(bubble);
      messages.appendChild(wrap);
      messages.scrollTop = messages.scrollHeight;
    }

    function setEvidence(blocks) {
      evidence.textContent = "";
      if (!blocks || blocks.length === 0) {
        const empty = document.createElement("p");
        empty.className = "evidence-empty";
        empty.textContent = "No compiled evidence returned.";
        evidence.appendChild(empty);
        return;
      }
      for (const block of blocks) {
        const item = document.createElement("div");
        item.className = "evidence-item";
        const title = document.createElement("div");
        title.className = "evidence-title";
        title.textContent = block.title || block.type;
        item.appendChild(title);
        if (block.type === "image") {
          const img = document.createElement("img");
          img.src = block.src;
          img.alt = block.title || "compiled diagram";
          item.appendChild(img);
        } else if (block.text) {
          const pre = document.createElement("pre");
          pre.textContent = block.text;
          item.appendChild(pre);
        }
        evidence.appendChild(item);
      }
    }

    async function loadSession(path) {
      specPath = path;
      const res = await fetch("/api/session?spec=" + encodeURIComponent(path));
      const data = await res.json();
      if (data.error) throw new Error(data.error);
      model.value = data.default_model || model.value;
      apiStatus.textContent = data.openai_configured ? "OpenAI connected" : "local compiler only";
      apiStatus.className = data.openai_configured ? "status ready" : "status local";
      filesEl.textContent = "";
      for (const [name, content] of Object.entries(data.files)) {
        const label = document.createElement("label");
        label.textContent = name;
        const area = document.createElement("textarea");
        area.dataset.path = name;
        area.value = content;
        filesEl.appendChild(label);
        filesEl.appendChild(area);
      }
    }

    function collectFiles() {
      const files = {};
      for (const area of filesEl.querySelectorAll("textarea")) {
        files[area.dataset.path] = area.value;
      }
      return files;
    }

    async function sendMessage(text) {
      send.disabled = true;
      addMessage("user", [{type: "text", text}]);
      try {
        const res = await fetch("/api/chat", {
          method: "POST",
          headers: {"Content-Type": "application/json"},
          body: JSON.stringify({message: text, model: model.value, spec_path: specPath, files: collectFiles()})
        });
        const data = await res.json();
        addMessage("assistant", data.blocks || [{type: "text", text: data.error || "No response"}]);
        setEvidence(data.evidence || []);
      } finally {
        send.disabled = false;
      }
    }

    async function saveCurrentFiles() {
      saveFiles.disabled = true;
      try {
        const res = await fetch("/api/save", {
          method: "POST",
          headers: {"Content-Type": "application/json"},
          body: JSON.stringify({files: collectFiles()})
        });
        const data = await res.json();
        if (data.error) throw new Error(data.error);
        addMessage("assistant", [{type: "text", text: "Saved files:\n" + data.saved.join("\n")}]);
      } catch (err) {
        addMessage("assistant", [{type: "text", text: "Save failed: " + err.message}]);
      } finally {
        saveFiles.disabled = false;
      }
    }

    example.addEventListener("change", () => loadSession(example.value));
    thread.addEventListener("change", () => {
      const stored = JSON.parse(localStorage.getItem("specweb.threads") || "[]");
      const item = stored.find(row => row.id === thread.value);
      if (!item) return;
      threadID = item.id;
      transcript = item.messages || [];
      redrawTranscript();
    });
    newThread.addEventListener("click", () => {
      const stored = JSON.parse(localStorage.getItem("specweb.threads") || "[]");
      const item = {id: "thread-" + Date.now(), title: "New design thread", messages: []};
      stored.unshift(item);
      localStorage.setItem("specweb.threads", JSON.stringify(stored));
      threadID = item.id;
      transcript = [];
      loadThreadOptionsOnly(stored);
      redrawTranscript();
      addMessage("assistant", [{type: "text", text: "Started a new design thread."}]);
    });
    saveFiles.addEventListener("click", saveCurrentFiles);
    for (const chip of document.querySelectorAll(".chip")) {
      chip.addEventListener("click", () => {
        prompt.value = chip.dataset.prompt;
        composer.requestSubmit();
      });
    }
    composer.addEventListener("submit", (event) => {
      event.preventDefault();
      const text = prompt.value.trim() || "Compile this and show me the diagrams.";
      prompt.value = "";
      sendMessage(text);
    });

    loadThreads();
    loadSession(example.value).then(() => {
      addMessage("assistant", [{type: "text", text: "Loaded the reservation example. Ask me to compile evidence, inspect a scenario, or discuss a protocol change."}]);
    }).catch(err => addMessage("assistant", [{type: "text", text: err.message}]));
  </script>
</body>
</html>`

func renderIndex() string {
	var b bytes.Buffer
	_ = indexTemplate.Execute(&b, nil)
	return b.String()
}
