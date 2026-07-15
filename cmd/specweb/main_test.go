package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionReturnsEditableSpecAndProtoFiles(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	app := newServer("../..")
	req := httptest.NewRequest(http.MethodGet, "/api/session?spec=examples/auth.convspec", nil)
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.SpecPath != "examples/auth.convspec" {
		t.Fatalf("spec path = %q", response.SpecPath)
	}
	if !strings.Contains(response.Files["examples/auth.convspec"], "conversation login") {
		t.Fatal("missing auth convspec")
	}
	if !strings.Contains(response.Files["examples/auth.proto"], "message LoginRequest") {
		t.Fatal("missing imported auth proto")
	}
	if response.OpenAIConfigured {
		t.Fatal("session should report OpenAI as unconfigured")
	}
	if response.DefaultModel == "" {
		t.Fatal("missing default model")
	}
	if len(response.AvailableExamples) < 4 {
		t.Fatalf("available examples = %d, want at least 4", len(response.AvailableExamples))
	}
}

func TestChatResponseIncludesTextAndEvidenceImages(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	app := newServer("../..")
	files, err := app.readSessionFiles("examples/reservation.convspec")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(chatRequest{
		Message:  "compile and show images",
		Model:    "test-model",
		SpecPath: "examples/reservation.convspec",
		Files:    files,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response chatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Blocks) < 3 {
		t.Fatalf("expected text blocks, got %#v", response.Blocks)
	}
	if response.Blocks[0].Type != "text" || !strings.Contains(response.Blocks[0].Text, "Compiled `reservation` successfully") {
		t.Fatalf("first block was not compile summary: %#v", response.Blocks[0])
	}
	if !strings.Contains(response.Blocks[0].Text, "inbox `supplier`") {
		t.Fatalf("compile summary did not include inbox metrics: %s", response.Blocks[0].Text)
	}
	var imageBlocks int
	for _, block := range response.Evidence {
		if block.Type == "image" {
			imageBlocks++
			if !strings.HasPrefix(block.Src, "data:image/png;base64,") && !strings.HasPrefix(block.Src, "data:image/svg+xml;base64,") {
				t.Fatalf("image block src is not inline image: %.40s", block.Src)
			}
		}
	}
	if imageBlocks != 10 {
		t.Fatalf("image blocks = %d, want state + three actor projections + six paths", imageBlocks)
	}
	if response.UsedLLM {
		t.Fatal("response should not use LLM without OPENAI_API_KEY")
	}
	if !strings.Contains(response.Blocks[len(response.Blocks)-1].Text, "OPENAI_API_KEY is not set") {
		t.Fatalf("missing OpenAI fallback message: %#v", response.Blocks)
	}
}

func TestExtractOpenAIText(t *testing.T) {
	text, err := extractOpenAIText([]byte(`{"output":[{"content":[{"text":"hello"},{"text":"world"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello\n\nworld" {
		t.Fatalf("text = %q", text)
	}
}

func TestSaveWritesEditableFiles(t *testing.T) {
	root := t.TempDir()
	app := newServer(root)
	payload, err := json.Marshal(saveRequest{Files: map[string]string{
		"examples/demo.convspec": "spec demo\n",
	}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/save", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "examples/demo.convspec"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "spec demo\n" {
		t.Fatalf("saved data = %q", data)
	}
}
