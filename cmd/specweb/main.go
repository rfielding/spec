package main

import (
	"bytes"
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
	SpecPath string            `json:"spec_path"`
	Files    map[string]string `json:"files"`
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
	writeJSON(w, http.StatusOK, sessionResponse{SpecPath: specPath, Files: files})
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
	SpecPath string            `json:"spec_path"`
	Files    map[string]string `json:"files"`
}

type chatResponse struct {
	Blocks []messageBlock `json:"blocks"`
}

type messageBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Title string `json:"title,omitempty"`
	Src   string `json:"src,omitempty"`
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
	response, err := s.respond(request)
	if err != nil {
		writeJSON(w, http.StatusOK, chatResponse{Blocks: []messageBlock{{Type: "text", Text: "Compilation failed:\n" + err.Error()}}})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) respond(request chatRequest) (chatResponse, error) {
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

	var blocks []messageBlock
	blocks = append(blocks, messageBlock{Type: "text", Text: compileSummary(spec, analysis)})
	for _, conversation := range images {
		blocks = append(blocks, messageBlock{
			Type:  "image",
			Title: conversation.Title + " state machine",
			Src:   conversation.StateImage.Src,
		})
		for _, image := range conversation.PathImages {
			blocks = append(blocks, messageBlock{
				Type:  "image",
				Title: conversation.Title + " " + image.Title,
				Src:   image.Src,
			})
		}
	}
	if strings.TrimSpace(request.Message) != "" {
		blocks = append(blocks, messageBlock{Type: "text", Text: "LLM integration is not wired yet. For now, this assistant deterministically compiles the edited files and returns the evidence the future LLM should cite: structural analysis plus rendered diagrams."})
	}
	return chatResponse{Blocks: blocks}, nil
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
			if assertion.Error != "" {
				fmt.Fprintf(&b, "  error: %s\n", assertion.Error)
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
    :root { font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #18202a; background: #eef2f6; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; }
    .shell { display: grid; grid-template-columns: minmax(360px, 42vw) 1fr; min-height: 100vh; }
    .editor { background: #f8fafc; border-right: 1px solid #d8dee7; padding: 18px; overflow: auto; }
    .chat { display: grid; grid-template-rows: auto 1fr auto; min-width: 0; }
    header { background: #fff; border-bottom: 1px solid #d8dee7; padding: 16px 20px; }
    h1 { font-size: 18px; margin: 0; }
    h2 { font-size: 14px; margin: 16px 0 8px; color: #334155; }
    label { display: block; font-size: 12px; font-weight: 650; color: #526070; margin: 12px 0 6px; }
    select, textarea, input { width: 100%; border: 1px solid #cbd5e1; border-radius: 6px; background: #fff; color: #18202a; }
    select, input { height: 34px; padding: 0 9px; }
    textarea { min-height: 260px; resize: vertical; padding: 10px; font: 13px/1.45 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
    .messages { padding: 22px; overflow: auto; }
    .message { max-width: 960px; margin: 0 auto 18px; display: grid; grid-template-columns: 42px 1fr; gap: 12px; }
    .avatar { width: 32px; height: 32px; border-radius: 50%; display: grid; place-items: center; font-weight: 700; font-size: 12px; background: #18202a; color: #fff; }
    .user .avatar { background: #2563eb; }
    .bubble { background: #fff; border: 1px solid #d8dee7; border-radius: 8px; padding: 14px; overflow: hidden; }
    .bubble pre { white-space: pre-wrap; margin: 0; font: 14px/1.5 inherit; }
    .bubble img { display: block; width: 100%; height: auto; border: 1px solid #e2e8f0; border-radius: 6px; margin-top: 8px; }
    .image-title { margin-top: 14px; font-size: 12px; color: #526070; font-weight: 650; }
    .composer { border-top: 1px solid #d8dee7; background: #fff; padding: 14px 20px; display: grid; grid-template-columns: 1fr auto; gap: 10px; }
    .composer textarea { min-height: 56px; max-height: 160px; }
    button { height: 40px; border: 0; border-radius: 6px; padding: 0 16px; background: #18202a; color: #fff; font-weight: 650; cursor: pointer; }
    button:disabled { opacity: .55; cursor: default; }
    .hint { color: #64748b; font-size: 13px; line-height: 1.4; }
    @media (max-width: 900px) { .shell { grid-template-columns: 1fr; } .editor { border-right: 0; border-bottom: 1px solid #d8dee7; } }
  </style>
</head>
<body>
  <div class="shell">
    <aside class="editor">
      <h1>Convspec Workbench</h1>
      <p class="hint">Edit the spec/proto text, then ask the assistant to compile or show diagrams. Images are generated by Go/Graphviz, not by the LLM.</p>
      <label for="example">Example</label>
      <select id="example">
        <option value="examples/reservation.convspec">reservation</option>
        <option value="examples/auth.convspec">auth</option>
      </select>
      <div id="files"></div>
    </aside>
    <section class="chat">
      <header><h1>Design Discussion</h1></header>
      <main id="messages" class="messages"></main>
      <form id="composer" class="composer">
        <textarea id="prompt" placeholder="Ask to compile, show diagrams, or critique the protocol shape."></textarea>
        <button id="send" type="submit">Send</button>
      </form>
    </section>
  </div>
  <script>
    const example = document.querySelector("#example");
    const filesEl = document.querySelector("#files");
    const messages = document.querySelector("#messages");
    const composer = document.querySelector("#composer");
    const prompt = document.querySelector("#prompt");
    const send = document.querySelector("#send");
    let specPath = example.value;

    function addMessage(role, blocks) {
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

    async function loadSession(path) {
      specPath = path;
      const res = await fetch("/api/session?spec=" + encodeURIComponent(path));
      const data = await res.json();
      if (data.error) throw new Error(data.error);
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
          body: JSON.stringify({message: text, spec_path: specPath, files: collectFiles()})
        });
        const data = await res.json();
        addMessage("assistant", data.blocks || [{type: "text", text: data.error || "No response"}]);
      } finally {
        send.disabled = false;
      }
    }

    example.addEventListener("change", () => loadSession(example.value));
    composer.addEventListener("submit", (event) => {
      event.preventDefault();
      const text = prompt.value.trim() || "Compile this and show me the diagrams.";
      prompt.value = "";
      sendMessage(text);
    });

    loadSession(example.value).then(() => {
      addMessage("assistant", [{type: "text", text: "Loaded the reservation example. Ask me to compile it and show diagrams."}]);
    }).catch(err => addMessage("assistant", [{type: "text", text: err.message}]));
  </script>
</body>
</html>`

func renderIndex() string {
	var b bytes.Buffer
	_ = indexTemplate.Execute(&b, nil)
	return b.String()
}
