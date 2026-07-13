package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgsAcceptsFlagsAfterSpec(t *testing.T) {
	args, err := parseArgs([]string{"examples/auth.convspec", "--format", "html", "-o", "build/auth.html"})
	if err != nil {
		t.Fatal(err)
	}
	if args.specPath != "examples/auth.convspec" {
		t.Fatalf("specPath = %q", args.specPath)
	}
	if args.format != "html" {
		t.Fatalf("format = %q", args.format)
	}
	if args.outputPath != "build/auth.html" {
		t.Fatalf("outputPath = %q", args.outputPath)
	}
}

func TestRunCreatesOutputDirectories(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	outputPath := filepath.Join(t.TempDir(), "nested", "auth.html")
	os.Args = []string{
		"convspec",
		"../../examples/auth.convspec",
		"--format",
		"html",
		"-o",
		outputPath,
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if !strings.Contains(html, "auth_assets/login_state.png") {
		t.Fatalf("HTML output did not reference state image:\n%s", html)
	}
	if !strings.Contains(html, "auth_assets/login_path_01.svg") {
		t.Fatalf("HTML output did not reference path image:\n%s", html)
	}
	for _, name := range []string{"login_state.png", "login_path_01.svg", "login_path_02.svg"} {
		imagePath := filepath.Join(filepath.Dir(outputPath), "auth_assets", name)
		info, err := os.Stat(imagePath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s is empty", imagePath)
		}
	}
}
