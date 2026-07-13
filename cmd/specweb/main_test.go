package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionReturnsEditableSpecAndProtoFiles(t *testing.T) {
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
}

func TestChatResponseIncludesTextAndInlineImages(t *testing.T) {
	app := newServer("../..")
	files, err := app.readSessionFiles("examples/auth.convspec")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(chatRequest{
		Message:  "compile and show images",
		SpecPath: "examples/auth.convspec",
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
		t.Fatalf("expected text plus image blocks, got %#v", response.Blocks)
	}
	if response.Blocks[0].Type != "text" || !strings.Contains(response.Blocks[0].Text, "Compiled `auth` successfully") {
		t.Fatalf("first block was not compile summary: %#v", response.Blocks[0])
	}
	var imageBlocks int
	for _, block := range response.Blocks {
		if block.Type == "image" {
			imageBlocks++
			if !strings.HasPrefix(block.Src, "data:image/png;base64,") && !strings.HasPrefix(block.Src, "data:image/svg+xml;base64,") {
				t.Fatalf("image block src is not inline image: %.40s", block.Src)
			}
		}
	}
	if imageBlocks != 3 {
		t.Fatalf("image blocks = %d, want state + two paths", imageBlocks)
	}
}
