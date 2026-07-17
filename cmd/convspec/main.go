package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rfielding/spec/internal/convspec"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "convspec: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	args, err := parseArgs(os.Args[1:])
	if err != nil {
		return err
	}
	if args.specPath == "" {
		return fmt.Errorf("usage: convspec [--format mermaid|mermaid-sequence|dot|json|html|checks|metrics|traffic] [-o file] spec.convspec")
	}

	spec, err := convspec.ParseFile(args.specPath)
	if err != nil {
		return err
	}

	var rendered string
	switch args.format {
	case "mermaid":
		rendered = convspec.EmitMermaid(spec)
	case "mermaid-sequence":
		rendered = convspec.EmitMermaidSequences(spec)
	case "dot":
		rendered = convspec.EmitDOT(spec)
	case "json":
		rendered, err = convspec.EmitJSON(spec)
		if err != nil {
			return err
		}
	case "checks":
		rendered = convspec.EmitChecks(spec)
	case "metrics":
		rendered = convspec.EmitMetrics(spec)
	case "traffic":
		rendered = convspec.EmitTraffic(spec)
	case "html":
		if args.outputPath != "" {
			return convspec.WriteHTMLReport(spec, args.outputPath)
		}
		rendered, err = convspec.EmitHTML(spec)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown format %q", args.format)
	}

	if args.outputPath != "" {
		if dir := filepath.Dir(args.outputPath); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		return os.WriteFile(args.outputPath, []byte(rendered), 0o644)
	}
	fmt.Print(rendered)
	return nil
}

type cliArgs struct {
	format     string
	outputPath string
	specPath   string
}

func parseArgs(raw []string) (cliArgs, error) {
	args := cliArgs{format: "mermaid"}
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case "--format":
			i++
			if i >= len(raw) {
				return cliArgs{}, fmt.Errorf("--format requires a value")
			}
			args.format = raw[i]
		case "-o", "--output":
			i++
			if i >= len(raw) {
				return cliArgs{}, fmt.Errorf("%s requires a value", raw[i-1])
			}
			args.outputPath = raw[i]
		default:
			if len(raw[i]) > 0 && raw[i][0] == '-' {
				return cliArgs{}, fmt.Errorf("unknown flag %s", raw[i])
			}
			if args.specPath != "" {
				return cliArgs{}, fmt.Errorf("only one spec file may be provided")
			}
			args.specPath = raw[i]
		}
	}
	return args, nil
}
