package convspec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ParseError struct {
	Path string
	Line int
	Msg  string
}

func (e ParseError) Error() string {
	if e.Line <= 0 {
		return fmt.Sprintf("%s: %s", e.Path, e.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Msg)
}

type lineReader struct {
	path  string
	lines []sourceLine
	pos   int
}

type sourceLine struct {
	num  int
	text string
}

func newLineReader(path string, data []byte) *lineReader {
	raw := strings.Split(string(data), "\n")
	lines := make([]sourceLine, 0, len(raw))
	for i, line := range raw {
		lines = append(lines, sourceLine{num: i + 1, text: strings.TrimSpace(line)})
	}
	return &lineReader{path: path, lines: lines}
}

func (r *lineReader) peek() (sourceLine, bool) {
	for r.pos < len(r.lines) {
		line := r.lines[r.pos]
		if line.text == "" || strings.HasPrefix(line.text, "#") || strings.HasPrefix(line.text, "//") {
			r.pos++
			continue
		}
		return line, true
	}
	return sourceLine{}, false
}

func (r *lineReader) pop() (sourceLine, error) {
	line, ok := r.peek()
	if !ok {
		return sourceLine{}, r.err(0, "unexpected end of file")
	}
	r.pos++
	return line, nil
}

func (r *lineReader) err(line int, msg string) error {
	return ParseError{Path: r.path, Line: line, Msg: msg}
}

func ParseFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := newLineReader(path, data)
	first, err := reader.pop()
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(first.text)
	if len(parts) != 2 || parts[0] != "spec" {
		return nil, reader.err(first.num, "expected: spec <name>")
	}

	spec := &Spec{Name: parts[1], SourcePath: cleanPath(path)}
	for {
		line, ok := reader.peek()
		if !ok {
			break
		}
		line, _ = reader.pop()
		switch {
		case strings.HasPrefix(line.text, "import "):
			importPath, err := parseImport(reader, line)
			if err != nil {
				return nil, err
			}
			spec.Imports = append(spec.Imports, importPath)
		case line.text == "participants":
			participants, err := parseParticipants(reader)
			if err != nil {
				return nil, err
			}
			spec.Participants = append(spec.Participants, participants...)
		case strings.HasPrefix(line.text, "conversation "):
			conversation, err := parseConversation(reader, line)
			if err != nil {
				return nil, err
			}
			spec.Conversations = append(spec.Conversations, conversation)
		default:
			return nil, reader.err(line.num, "unexpected top-level statement: "+line.text)
		}
	}

	baseDir := filepath.Dir(path)
	for _, importPath := range spec.Imports {
		protoPath := filepath.Join(baseDir, importPath)
		protoFile, err := ParseProtoFile(protoPath)
		if err != nil {
			return nil, fmt.Errorf("%s: imported proto %q: %w", path, importPath, err)
		}
		spec.ProtoFiles = append(spec.ProtoFiles, protoFile)
	}
	spec.buildMessageIndex()
	if err := Validate(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func parseImport(reader *lineReader, line sourceLine) (string, error) {
	value := strings.TrimSpace(strings.TrimPrefix(line.text, "import "))
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", reader.err(line.num, `expected: import "file.proto"`)
	}
	return value[1 : len(value)-1], nil
}

func parseParticipants(reader *lineReader) ([]string, error) {
	var participants []string
	for {
		line, ok := reader.peek()
		if !ok {
			break
		}
		if strings.HasPrefix(line.text, "conversation ") ||
			strings.HasPrefix(line.text, "import ") ||
			line.text == "participants" ||
			strings.HasPrefix(line.text, "spec ") {
			break
		}
		line, _ = reader.pop()
		if strings.Contains(line.text, " ") || line.text == "{" || line.text == "}" {
			return nil, reader.err(line.num, "invalid participant: "+line.text)
		}
		participants = append(participants, line.text)
	}
	if len(participants) == 0 {
		return nil, reader.err(0, "participants block must contain at least one participant")
	}
	return participants, nil
}

func parseConversation(reader *lineReader, header sourceLine) (Conversation, error) {
	if !strings.HasSuffix(header.text, "{") {
		return Conversation{}, reader.err(header.num, "conversation header must end with {")
	}
	parts := strings.Fields(strings.TrimSpace(strings.TrimSuffix(header.text, "{")))
	if len(parts) != 2 && len(parts) != 4 {
		return Conversation{}, reader.err(header.num, "expected: conversation <name> [version <n>] {")
	}
	if parts[0] != "conversation" {
		return Conversation{}, reader.err(header.num, "expected: conversation <name> [version <n>] {")
	}
	conversation := Conversation{Name: parts[1], States: map[string]State{}}
	if len(parts) == 4 {
		if parts[2] != "version" {
			return Conversation{}, reader.err(header.num, "expected: conversation <name> version <n> {")
		}
		conversation.Version = parts[3]
	}
	for {
		line, err := reader.pop()
		if err != nil {
			return Conversation{}, reader.err(header.num, "conversation "+conversation.Name+" is missing closing }")
		}
		switch {
		case line.text == "}":
			if conversation.Start == "" {
				return Conversation{}, reader.err(line.num, "conversation "+conversation.Name+" is missing start")
			}
			return conversation, nil
		case strings.HasPrefix(line.text, "start "):
			conversation.Start = strings.TrimSpace(strings.TrimPrefix(line.text, "start "))
		case strings.HasPrefix(line.text, "state "):
			state, err := parseState(reader, line)
			if err != nil {
				return Conversation{}, err
			}
			if _, exists := conversation.States[state.Name]; exists {
				return Conversation{}, reader.err(line.num, "duplicate state: "+state.Name)
			}
			conversation.States[state.Name] = state
			conversation.Order = append(conversation.Order, state.Name)
		default:
			return Conversation{}, reader.err(line.num, "unexpected conversation statement: "+line.text)
		}
	}
}

func parseState(reader *lineReader, header sourceLine) (State, error) {
	hasBody := strings.HasSuffix(header.text, "{")
	text := strings.TrimSpace(strings.TrimSuffix(header.text, "{"))
	parts := strings.Fields(text)
	if len(parts) != 2 && len(parts) != 3 || parts[0] != "state" {
		return State{}, reader.err(header.num, "expected: state <name> [accept|reject] [{]")
	}
	state := State{Name: parts[1]}
	if len(parts) == 3 {
		if parts[2] != "accept" && parts[2] != "reject" {
			return State{}, reader.err(header.num, "state terminal marker must be accept or reject")
		}
		state.Terminal = parts[2]
	}
	if !hasBody {
		return state, nil
	}

	current := -1
	for {
		line, err := reader.pop()
		if err != nil {
			return State{}, reader.err(header.num, "state "+state.Name+" is missing closing }")
		}
		switch {
		case line.text == "}":
			if current >= 0 && state.Transitions[current].Target == "" {
				return State{}, reader.err(line.num, "transition is missing goto")
			}
			return state, nil
		case strings.HasPrefix(line.text, "emits "):
			state.Emits = append(state.Emits, strings.TrimSpace(strings.TrimPrefix(line.text, "emits ")))
		case strings.HasPrefix(line.text, "on "):
			if current >= 0 && state.Transitions[current].Target == "" {
				return State{}, reader.err(line.num, "transition is missing goto before next on")
			}
			transition, err := parseTransitionHeader(reader, line)
			if err != nil {
				return State{}, err
			}
			state.Transitions = append(state.Transitions, transition)
			current = len(state.Transitions) - 1
		default:
			if current < 0 {
				return State{}, reader.err(line.num, "expected emits or on statement, got: "+line.text)
			}
			transition := &state.Transitions[current]
			switch {
			case strings.HasPrefix(line.text, "bind "):
				transition.Bind = strings.TrimSpace(strings.TrimPrefix(line.text, "bind "))
			case strings.HasPrefix(line.text, "when "):
				transition.Guards = append(transition.Guards, strings.TrimSpace(strings.TrimPrefix(line.text, "when ")))
			case strings.HasPrefix(line.text, "goto "):
				transition.Target = strings.TrimSpace(strings.TrimPrefix(line.text, "goto "))
			default:
				return State{}, reader.err(line.num, "unexpected transition statement: "+line.text)
			}
		}
	}
}

func parseTransitionHeader(reader *lineReader, line sourceLine) (Transition, error) {
	parts := strings.Fields(line.text)
	if len(parts) != 5 || parts[0] != "on" || parts[2] != "->" {
		return Transition{}, reader.err(line.num, "expected: on <sender> -> <receiver> <MessageType>")
	}
	return Transition{Sender: parts[1], Receiver: parts[3], MessageType: parts[4]}, nil
}
