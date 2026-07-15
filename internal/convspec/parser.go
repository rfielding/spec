package convspec

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
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
	if looksLikeLisp(data) {
		spec, err := parseLispSpec(path, data)
		if err != nil {
			return nil, err
		}
		return finishParsedSpec(spec, path)
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
		case line.text == "reliability":
			reliability, err := parseReliability(reader)
			if err != nil {
				return nil, err
			}
			spec.Reliability = append(spec.Reliability, reliability...)
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

	return finishParsedSpec(spec, path)
}

func looksLikeLisp(data []byte) bool {
	text := string(data)
	for i := 0; i < len(text); {
		switch {
		case unicode.IsSpace(rune(text[i])):
			i++
		case text[i] == ';' || text[i] == '#':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case text[i] == '/' && i+1 < len(text) && text[i+1] == '/':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		default:
			return text[i] == '('
		}
	}
	return false
}

func finishParsedSpec(spec *Spec, path string) (*Spec, error) {
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
			line.text == "reliability" ||
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

func parseReliability(reader *lineReader) ([]ReliabilitySpec, error) {
	var specs []ReliabilitySpec
	for {
		line, ok := reader.peek()
		if !ok {
			break
		}
		if strings.HasPrefix(line.text, "conversation ") ||
			strings.HasPrefix(line.text, "import ") ||
			line.text == "participants" ||
			line.text == "reliability" ||
			strings.HasPrefix(line.text, "spec ") {
			break
		}
		line, _ = reader.pop()
		parts := strings.Fields(line.text)
		if len(parts) < 2 {
			return nil, reader.err(line.num, "expected reliability entry: <actor> <availability>|parallel <availability>...")
		}
		spec := ReliabilitySpec{Actor: parts[0]}
		if parts[1] == "parallel" {
			if len(parts) < 3 {
				return nil, reader.err(line.num, "parallel reliability requires at least one replica availability")
			}
			for _, part := range parts[2:] {
				value, err := strconv.ParseFloat(part, 64)
				if err != nil {
					return nil, reader.err(line.num, "invalid reliability value")
				}
				spec.Parallel = append(spec.Parallel, value)
			}
		} else {
			if len(parts) != 2 {
				return nil, reader.err(line.num, "expected reliability entry: <actor> <availability>|parallel <availability>...")
			}
			value, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return nil, reader.err(line.num, "invalid reliability value")
			}
			spec.Availability = value
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return nil, reader.err(0, "reliability block must contain at least one entry")
	}
	return specs, nil
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
		case strings.HasPrefix(line.text, "assert "):
			assertion, err := parseAssertion(reader, line)
			if err != nil {
				return Conversation{}, err
			}
			conversation.Asserts = append(conversation.Asserts, assertion)
		case strings.HasPrefix(line.text, "queue "):
			queue, err := parseQueue(reader, line)
			if err != nil {
				return Conversation{}, err
			}
			conversation.Queues = append(conversation.Queues, queue)
		case strings.HasPrefix(line.text, "inbox "):
			queue, err := parseInbox(reader, line)
			if err != nil {
				return Conversation{}, err
			}
			conversation.Queues = append(conversation.Queues, queue)
		default:
			return Conversation{}, reader.err(line.num, "unexpected conversation statement: "+line.text)
		}
	}
}

func parseAssertion(reader *lineReader, line sourceLine) (Assertion, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line.text, "assert "))
	name, formula, ok := strings.Cut(rest, ":")
	if !ok {
		return Assertion{}, reader.err(line.num, "expected: assert <name>: <CTL formula>")
	}
	name = strings.TrimSpace(name)
	formula = strings.TrimSpace(formula)
	if name == "" || formula == "" {
		return Assertion{}, reader.err(line.num, "assertion name and formula are required")
	}
	if strings.Contains(name, " ") {
		return Assertion{}, reader.err(line.num, "assertion name must not contain spaces")
	}
	return Assertion{Name: name, Formula: formula}, nil
}

func parseInbox(reader *lineReader, header sourceLine) (QueueSpec, error) {
	if !strings.HasSuffix(header.text, "{") {
		return QueueSpec{}, reader.err(header.num, "inbox header must end with {")
	}
	parts := strings.Fields(strings.TrimSpace(strings.TrimSuffix(header.text, "{")))
	if len(parts) != 2 || parts[0] != "inbox" {
		return QueueSpec{}, reader.err(header.num, "expected: inbox <actor> {")
	}
	queue := QueueSpec{Name: parts[1], Actor: parts[1], Kind: "inbox"}
	for {
		line, err := reader.pop()
		if err != nil {
			return QueueSpec{}, reader.err(header.num, "inbox "+queue.Name+" is missing closing }")
		}
		if line.text == "}" {
			if queue.Capacity <= 0 {
				return QueueSpec{}, reader.err(line.num, "inbox capacity must be greater than zero")
			}
			return queue, nil
		}
		parts := strings.Fields(line.text)
		if len(parts) != 2 {
			return QueueSpec{}, reader.err(line.num, "expected inbox property: <name> <value>")
		}
		switch parts[0] {
		case "capacity":
			value, err := strconv.Atoi(parts[1])
			if err != nil {
				return QueueSpec{}, reader.err(line.num, "invalid capacity")
			}
			queue.Capacity = value
		default:
			return QueueSpec{}, reader.err(line.num, "unknown inbox property: "+parts[0])
		}
	}
}

func parseQueue(reader *lineReader, header sourceLine) (QueueSpec, error) {
	if !strings.HasSuffix(header.text, "{") {
		return QueueSpec{}, reader.err(header.num, "queue header must end with {")
	}
	parts := strings.Fields(strings.TrimSpace(strings.TrimSuffix(header.text, "{")))
	if len(parts) != 2 || parts[0] != "queue" {
		return QueueSpec{}, reader.err(header.num, "expected: queue <name> {")
	}
	queue := QueueSpec{Name: parts[1], Kind: "queue"}
	for {
		line, err := reader.pop()
		if err != nil {
			return QueueSpec{}, reader.err(header.num, "queue "+queue.Name+" is missing closing }")
		}
		if line.text == "}" {
			if queue.Capacity <= 0 {
				return QueueSpec{}, reader.err(line.num, "queue capacity must be greater than zero")
			}
			return queue, nil
		}
		parts := strings.Fields(line.text)
		if len(parts) != 2 {
			return QueueSpec{}, reader.err(line.num, "expected queue property: <name> <value>")
		}
		switch parts[0] {
		case "arrival_rate":
			value, err := strconv.ParseFloat(strings.TrimSuffix(parts[1], "/s"), 64)
			if err != nil {
				return QueueSpec{}, reader.err(line.num, "invalid arrival_rate")
			}
			queue.ArrivalRate = value
		case "service_time_ms":
			value, err := strconv.ParseFloat(strings.TrimSuffix(parts[1], "ms"), 64)
			if err != nil {
				return QueueSpec{}, reader.err(line.num, "invalid service_time_ms")
			}
			queue.ServiceTimeMS = value
		case "capacity":
			value, err := strconv.Atoi(parts[1])
			if err != nil {
				return QueueSpec{}, reader.err(line.num, "invalid capacity")
			}
			queue.Capacity = value
		case "workers":
			return QueueSpec{}, reader.err(line.num, "queue workers is no longer a queue property; model drain/service with actor messages")
		default:
			return QueueSpec{}, reader.err(line.num, "unknown queue property: "+parts[0])
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
	branchBaseGuards := map[int]int{}
	for {
		line, err := reader.pop()
		if err != nil {
			return State{}, reader.err(header.num, "state "+state.Name+" is missing closing }")
		}
		switch {
		case line.text == "}":
			if current >= 0 && state.Transitions[current].Target == "" {
				return State{}, reader.err(line.num, "transition is missing then/goto")
			}
			return state, nil
		case strings.HasPrefix(line.text, "emits "):
			state.Emits = append(state.Emits, strings.TrimSpace(strings.TrimPrefix(line.text, "emits ")))
		case strings.HasPrefix(line.text, "holds "):
			state.Emits = append(state.Emits, strings.TrimSpace(strings.TrimPrefix(line.text, "holds ")))
		case strings.HasPrefix(line.text, "state_is "):
			state.Emits = append(state.Emits, strings.TrimSpace(strings.TrimPrefix(line.text, "state_is ")))
		case strings.HasPrefix(line.text, "on "):
			if current >= 0 && state.Transitions[current].Target == "" {
				return State{}, reader.err(line.num, "transition is missing then/goto before next on")
			}
			transition, err := parseTransitionHeader(reader, line)
			if err != nil {
				return State{}, err
			}
			state.Transitions = append(state.Transitions, transition)
			current = len(state.Transitions) - 1
		default:
			if current < 0 {
				return State{}, reader.err(line.num, "expected state_is/holds/emits or on statement, got: "+line.text)
			}
			transition := &state.Transitions[current]
			switch {
			case strings.HasPrefix(line.text, "bind "):
				transition.Bind = strings.TrimSpace(strings.TrimPrefix(line.text, "bind "))
			case strings.HasPrefix(line.text, "when "):
				if strings.Contains(line.text, " then ") {
					next, err := parseWhenThen(reader, line, state.Transitions, current, branchBaseGuards)
					if err != nil {
						return State{}, err
					}
					state.Transitions = next.transitions
					current = next.current
				} else {
					transition.Guards = append(transition.Guards, strings.TrimSpace(strings.TrimPrefix(line.text, "when ")))
				}
			case strings.HasPrefix(line.text, "chance "):
				if err := parseChance(reader, line, transition, strings.TrimSpace(strings.TrimPrefix(line.text, "chance "))); err != nil {
					return State{}, err
				}
			case strings.HasPrefix(line.text, "latency_ms "):
				value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line.text, "latency_ms ")), 64)
				if err != nil {
					return State{}, reader.err(line.num, "invalid latency_ms value")
				}
				transition.DwellTimeMS = &value
				transition.LatencyMS = &value
			case strings.HasPrefix(line.text, "dwell_time_ms "):
				value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line.text, "dwell_time_ms ")), 64)
				if err != nil {
					return State{}, reader.err(line.num, "invalid dwell_time_ms value")
				}
				transition.DwellTimeMS = &value
			case strings.HasPrefix(line.text, "bytes "):
				value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line.text, "bytes ")), 64)
				if err != nil {
					return State{}, reader.err(line.num, "invalid bytes value")
				}
				transition.Bytes = &value
			case strings.HasPrefix(line.text, "queue "):
				transition.Queue = strings.TrimSpace(strings.TrimPrefix(line.text, "queue "))
			case strings.HasPrefix(line.text, "goto "):
				if err := parseTransitionTarget(reader, line, "goto", transition); err != nil {
					return State{}, err
				}
			case strings.HasPrefix(line.text, "then "):
				if transition.Target != "" {
					cloned := *transition
					cloned.Target = ""
					cloned.Chance = nil
					cloned.Otherwise = false
					cloned.Guards = append([]string(nil), transition.Guards...)
					state.Transitions = append(state.Transitions, cloned)
					current = len(state.Transitions) - 1
					transition = &state.Transitions[current]
				}
				if err := parseTransitionTarget(reader, line, "then", transition); err != nil {
					return State{}, err
				}
			default:
				return State{}, reader.err(line.num, "unexpected transition statement: "+line.text)
			}
		}
	}
}

type branchParseResult struct {
	transitions []Transition
	current     int
}

func parseWhenThen(reader *lineReader, line sourceLine, transitions []Transition, current int, branchBaseGuards map[int]int) (branchParseResult, error) {
	transition := &transitions[current]
	if _, ok := branchBaseGuards[current]; !ok {
		branchBaseGuards[current] = len(transition.Guards)
	}
	if transition.Target != "" {
		baseCount := branchBaseGuards[current]
		cloned := *transition
		cloned.Target = ""
		cloned.Chance = nil
		cloned.Otherwise = false
		cloned.Guards = append([]string(nil), transition.Guards[:baseCount]...)
		transitions = append(transitions, cloned)
		current = len(transitions) - 1
		branchBaseGuards[current] = baseCount
		transition = &transitions[current]
	}
	parts := strings.Fields(line.text)
	thenIndex := -1
	for i, part := range parts {
		if part == "then" {
			thenIndex = i
			break
		}
	}
	if thenIndex <= 1 || thenIndex+1 >= len(parts) {
		return branchParseResult{}, reader.err(line.num, "expected: when <guard> then <state> [chance <probability>]")
	}
	transition.Guards = append(transition.Guards, strings.Join(parts[1:thenIndex], " "))
	transition.Target = parts[thenIndex+1]
	if thenIndex+2 < len(parts) {
		if thenIndex+4 != len(parts) || parts[thenIndex+2] != "chance" {
			return branchParseResult{}, reader.err(line.num, "expected: when <guard> then <state> chance <probability>")
		}
		if err := parseChance(reader, line, transition, parts[thenIndex+3]); err != nil {
			return branchParseResult{}, err
		}
	}
	return branchParseResult{transitions: transitions, current: current}, nil
}

func parseChance(reader *lineReader, line sourceLine, transition *Transition, valueText string) error {
	if valueText == "otherwise" {
		transition.Chance = nil
		transition.Otherwise = true
		return nil
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil {
		return reader.err(line.num, "invalid chance value")
	}
	transition.Chance = &value
	transition.Otherwise = false
	return nil
}

func parseTransitionHeader(reader *lineReader, line sourceLine) (Transition, error) {
	parts := strings.Fields(line.text)
	if len(parts) != 5 || parts[0] != "on" || parts[2] != "->" {
		return Transition{}, reader.err(line.num, "expected: on <sender> -> <receiver> <MessageType>")
	}
	return Transition{Sender: parts[1], Receiver: parts[3], MessageType: parts[4]}, nil
}

func parseTransitionTarget(reader *lineReader, line sourceLine, keyword string, transition *Transition) error {
	parts := strings.Fields(line.text)
	if len(parts) != 2 && len(parts) != 4 {
		return reader.err(line.num, "expected: "+keyword+" <state> [chance <probability>]")
	}
	if parts[0] != keyword {
		return reader.err(line.num, "expected: "+keyword+" <state> [chance <probability>]")
	}
	transition.Target = parts[1]
	if len(parts) == 4 {
		if parts[2] != "chance" {
			return reader.err(line.num, "expected: "+keyword+" <state> chance <probability>")
		}
		if err := parseChance(reader, line, transition, parts[3]); err != nil {
			return err
		}
	}
	return nil
}
