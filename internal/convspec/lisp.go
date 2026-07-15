package convspec

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type lispNode struct {
	Atom   string
	String bool
	List   []lispNode
	Line   int
}

type lispToken struct {
	text   string
	string bool
	line   int
}

func parseLispSpec(path string, data []byte) (*Spec, error) {
	tokens, err := tokenizeLisp(path, string(data))
	if err != nil {
		return nil, err
	}
	nodes, err := parseLispNodes(path, tokens)
	if err != nil {
		return nil, err
	}
	if len(nodes) != 1 {
		return nil, ParseError{Path: path, Msg: "expected exactly one top-level (spec ...) form"}
	}
	return buildLispSpec(path, nodes[0])
}

func tokenizeLisp(path, text string) ([]lispToken, error) {
	var tokens []lispToken
	line := 1
	for i := 0; i < len(text); {
		ch := text[i]
		switch {
		case ch == '\n':
			line++
			i++
		case unicode.IsSpace(rune(ch)):
			i++
		case ch == ';' || ch == '#':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case ch == '/' && i+1 < len(text) && text[i+1] == '/':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case ch == '(' || ch == ')':
			tokens = append(tokens, lispToken{text: string(ch), line: line})
			i++
		case ch == '"':
			startLine := line
			i++
			var b strings.Builder
			closed := false
			for i < len(text) {
				if text[i] == '\n' {
					line++
				}
				if text[i] == '"' {
					i++
					tokens = append(tokens, lispToken{text: b.String(), string: true, line: startLine})
					closed = true
					break
				}
				if text[i] == '\\' && i+1 < len(text) {
					switch text[i+1] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					case '"', '\\':
						b.WriteByte(text[i+1])
					default:
						b.WriteByte(text[i+1])
					}
					i += 2
					continue
				}
				b.WriteByte(text[i])
				i++
			}
			if !closed {
				return nil, ParseError{Path: path, Line: startLine, Msg: "unterminated string"}
			}
		default:
			start := i
			startLine := line
			for i < len(text) && !unicode.IsSpace(rune(text[i])) && text[i] != '(' && text[i] != ')' {
				i++
			}
			tokens = append(tokens, lispToken{text: text[start:i], line: startLine})
		}
	}
	return tokens, nil
}

func parseLispNodes(path string, tokens []lispToken) ([]lispNode, error) {
	pos := 0
	var parse func() (lispNode, error)
	parse = func() (lispNode, error) {
		if pos >= len(tokens) {
			return lispNode{}, ParseError{Path: path, Msg: "unexpected end of Lisp form"}
		}
		token := tokens[pos]
		pos++
		if token.text == "(" {
			node := lispNode{Line: token.line}
			for {
				if pos >= len(tokens) {
					return lispNode{}, ParseError{Path: path, Line: token.line, Msg: "missing )"}
				}
				if tokens[pos].text == ")" {
					pos++
					return node, nil
				}
				child, err := parse()
				if err != nil {
					return lispNode{}, err
				}
				node.List = append(node.List, child)
			}
		}
		if token.text == ")" {
			return lispNode{}, ParseError{Path: path, Line: token.line, Msg: "unexpected )"}
		}
		return lispNode{Atom: token.text, String: token.string, Line: token.line}, nil
	}
	var nodes []lispNode
	for pos < len(tokens) {
		node, err := parse()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func buildLispSpec(path string, node lispNode) (*Spec, error) {
	if !node.isListHead("spec") || len(node.List) < 2 || !node.List[1].isAtom() {
		return nil, lispErr(path, node, "expected: (spec <name> ...)")
	}
	spec := &Spec{Name: node.List[1].Atom, SourcePath: cleanPath(path)}
	for _, form := range node.List[2:] {
		switch form.head() {
		case "import":
			if len(form.List) != 2 || !form.List[1].String {
				return nil, lispErr(path, form, `expected: (import "file.proto")`)
			}
			spec.Imports = append(spec.Imports, form.List[1].Atom)
		case "include":
			if len(form.List) != 2 || !form.List[1].String {
				return nil, lispErr(path, form, `expected: (include "file.convspec")`)
			}
			spec.Includes = append(spec.Includes, form.List[1].Atom)
		case "participants":
			return nil, lispErr(path, form, "participants is no longer a top-level form; use (actor <name> (capacity <n>))")
		case "actor":
			actor, err := buildLispActorSpec(path, form)
			if err != nil {
				return nil, err
			}
			spec.Actors = append(spec.Actors, actor)
		case "inbox":
			return nil, lispErr(path, form, "inbox is not a top-level form; use (actor <name> (capacity <n>))")
		case "reliability":
			reliability, err := buildLispReliability(path, form)
			if err != nil {
				return nil, err
			}
			spec.Reliability = append(spec.Reliability, reliability...)
		case "assert":
			assertion, err := buildLispAssertion(path, form)
			if err != nil {
				return nil, err
			}
			spec.Asserts = append(spec.Asserts, assertion)
		case "conversation":
			conversation, err := buildLispConversation(path, form)
			if err != nil {
				return nil, err
			}
			spec.Conversations = append(spec.Conversations, conversation)
		default:
			return nil, lispErr(path, form, "unexpected top-level form: "+form.head())
		}
	}
	return spec, nil
}

func parseLispConversationFragment(path string, data []byte) ([]Conversation, error) {
	tokens, err := tokenizeLisp(path, string(data))
	if err != nil {
		return nil, err
	}
	nodes, err := parseLispNodes(path, tokens)
	if err != nil {
		return nil, err
	}
	var conversations []Conversation
	for _, node := range nodes {
		switch node.head() {
		case "conversation":
			conversation, err := buildLispConversation(path, node)
			if err != nil {
				return nil, err
			}
			conversations = append(conversations, conversation)
		case "spec":
			if len(nodes) != 1 {
				return nil, lispErr(path, node, "spec fragment must be the only top-level form")
			}
			if len(node.List) < 2 || !node.List[1].isAtom() {
				return nil, lispErr(path, node, "expected: (spec <name> ...)")
			}
			for _, child := range node.List[2:] {
				if child.head() != "conversation" {
					return nil, lispErr(path, child, "included spec fragments may only contain conversation forms")
				}
				conversation, err := buildLispConversation(path, child)
				if err != nil {
					return nil, err
				}
				conversations = append(conversations, conversation)
			}
		default:
			return nil, lispErr(path, node, "included files may only contain conversation forms")
		}
	}
	if len(conversations) == 0 {
		return nil, ParseError{Path: path, Msg: "included file has no conversations"}
	}
	return conversations, nil
}

func buildLispActorSpec(path string, form lispNode) (ActorSpec, error) {
	if len(form.List) < 2 || !form.List[1].isAtom() {
		return ActorSpec{}, lispErr(path, form, "expected: (actor <name> (capacity <n>))")
	}
	actor := ActorSpec{Name: form.List[1].Atom}
	for _, child := range form.List[2:] {
		if child.isListHead("capacity") && len(child.List) == 2 {
			value, err := parseLispInt(path, child.List[1], "invalid actor capacity")
			if err != nil {
				return ActorSpec{}, err
			}
			actor.Capacity = value
			continue
		}
		return ActorSpec{}, lispErr(path, child, "unknown actor property: "+child.head())
	}
	if actor.Capacity <= 0 {
		return ActorSpec{}, lispErr(path, form, "actor capacity must be greater than zero")
	}
	return actor, nil
}

func buildLispReliability(path string, form lispNode) ([]ReliabilitySpec, error) {
	var specs []ReliabilitySpec
	for _, entry := range form.List[1:] {
		if len(entry.List) != 2 || !entry.List[0].isAtom() {
			return nil, lispErr(path, entry, "expected reliability entry: (<actor> <availability>|(parallel ...))")
		}
		spec := ReliabilitySpec{Actor: entry.List[0].Atom}
		if entry.List[1].isListHead("parallel") {
			for _, valueNode := range entry.List[1].List[1:] {
				value, err := parseLispFloat(path, valueNode, "invalid reliability value")
				if err != nil {
					return nil, err
				}
				spec.Parallel = append(spec.Parallel, value)
			}
		} else {
			value, err := parseLispFloat(path, entry.List[1], "invalid reliability value")
			if err != nil {
				return nil, err
			}
			spec.Availability = value
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func buildLispConversation(path string, form lispNode) (Conversation, error) {
	if len(form.List) < 2 || !form.List[1].isAtom() {
		return Conversation{}, lispErr(path, form, "expected: (conversation <name> ...)")
	}
	conversation := Conversation{Name: form.List[1].Atom, States: map[string]State{}}
	for _, child := range form.List[2:] {
		switch child.head() {
		case "version":
			if len(child.List) != 2 || !child.List[1].isAtom() {
				return Conversation{}, lispErr(path, child, "expected: (version <n>)")
			}
			conversation.Version = child.List[1].Atom
		case "start":
			if len(child.List) != 4 || !child.List[1].isAtom() || !child.List[2].isAtom() || !child.List[3].isAtom() {
				return Conversation{}, lispErr(path, child, "expected: (start <actor> <MessageType> <state>)")
			}
			conversation.StartActor = child.List[1].Atom
			conversation.StartMessage = child.List[2].Atom
			conversation.Start = child.List[3].Atom
		case "assert":
			assertion, err := buildLispAssertion(path, child)
			if err != nil {
				return Conversation{}, err
			}
			conversation.Asserts = append(conversation.Asserts, assertion)
		case "inbox":
			return Conversation{}, lispErr(path, child, "inbox is not a conversation form; actor capacity is declared with top-level actor")
		case "queue":
			return Conversation{}, lispErr(path, child, "queue is not a conversation form; every actor has one top-level inbox")
		case "metric":
			metric, err := buildLispMetric(path, child)
			if err != nil {
				return Conversation{}, err
			}
			conversation.Metrics = append(conversation.Metrics, metric)
		case "state":
			state, err := buildLispState(path, child, "")
			if err != nil {
				return Conversation{}, err
			}
			if err := addLispState(path, &conversation, child, state); err != nil {
				return Conversation{}, err
			}
		case "actor":
			if len(child.List) < 2 || !child.List[1].isAtom() {
				return Conversation{}, lispErr(path, child, "expected: (actor <name> ...)")
			}
			actor := child.List[1].Atom
			for _, actorChild := range child.List[2:] {
				if actorChild.head() != "state" {
					return Conversation{}, lispErr(path, actorChild, "actor form currently accepts state forms")
				}
				state, err := buildLispState(path, actorChild, actor)
				if err != nil {
					return Conversation{}, err
				}
				if err := addLispState(path, &conversation, actorChild, state); err != nil {
					return Conversation{}, err
				}
			}
		default:
			return Conversation{}, lispErr(path, child, "unexpected conversation form: "+child.head())
		}
	}
	if conversation.Start == "" {
		return Conversation{}, lispErr(path, form, "conversation "+conversation.Name+" is missing start")
	}
	return conversation, nil
}

func addLispState(path string, conversation *Conversation, form lispNode, state State) error {
	if _, exists := conversation.States[state.Name]; exists {
		return lispErr(path, form, "duplicate state: "+state.Name)
	}
	conversation.States[state.Name] = state
	conversation.Order = append(conversation.Order, state.Name)
	return nil
}

func buildLispAssertion(path string, form lispNode) (Assertion, error) {
	if len(form.List) != 3 || !form.List[1].isAtom() {
		return Assertion{}, lispErr(path, form, "expected: (assert <name> <formula>)")
	}
	formula, err := lispExprString(path, form.List[2])
	if err != nil {
		return Assertion{}, err
	}
	return Assertion{Name: form.List[1].Atom, Formula: formula}, nil
}

func buildLispMetric(path string, form lispNode) (MetricSpec, error) {
	if len(form.List) < 2 || !form.List[1].isAtom() {
		return MetricSpec{}, lispErr(path, form, "expected: (metric <name> ...)")
	}
	metric := MetricSpec{Name: form.List[1].Atom}
	for _, child := range form.List[2:] {
		if len(child.List) != 2 || !child.List[1].isAtom() {
			return MetricSpec{}, lispErr(path, child, "expected metric property: (<property> <value>)")
		}
		value := child.List[1].Atom
		switch child.head() {
		case "chart":
			if value != "line" && value != "pie" && value != "bar" {
				return MetricSpec{}, lispErr(path, child, "metric chart must be line, pie, or bar")
			}
			metric.Chart = value
		case "message":
			metric.Message = value
		case "value":
			metric.Value = value
		case "group_by":
			metric.GroupBy = value
		case "window":
			metric.Window = value
		case "reducer":
			metric.Reducer = value
		default:
			return MetricSpec{}, lispErr(path, child, "unknown metric property: "+child.head())
		}
	}
	if metric.Chart == "" {
		return MetricSpec{}, lispErr(path, form, "metric requires (chart line|pie|bar)")
	}
	return metric, nil
}

func buildLispState(path string, form lispNode, actor string) (State, error) {
	if len(form.List) < 2 || !form.List[1].isAtom() {
		return State{}, lispErr(path, form, "expected: (state <name> ...)")
	}
	state := State{Name: form.List[1].Atom, Actor: actor}
	for _, child := range form.List[2:] {
		if child.isAtom() {
			if child.Atom != "accept" && child.Atom != "reject" {
				return State{}, lispErr(path, child, "state terminal marker must be accept or reject")
			}
			state.Terminal = child.Atom
			continue
		}
		switch child.head() {
		case "state_is":
			if len(child.List) != 2 || !child.List[1].isAtom() {
				return State{}, lispErr(path, child, "expected: (state_is <prop>)")
			}
			state.StateIs = append(state.StateIs, child.List[1].Atom)
		case "on":
			transitions, err := buildLispOn(path, child, actor)
			if err != nil {
				return State{}, err
			}
			state.Transitions = append(state.Transitions, transitions...)
		default:
			return State{}, lispErr(path, child, "unexpected state form: "+child.head())
		}
	}
	return state, nil
}

func buildLispOn(path string, form lispNode, actor string) ([]Transition, error) {
	if len(form.List) < 2 || !form.List[1].isAtom() {
		return nil, lispErr(path, form, "expected: (on <MessageType> ...)")
	}
	base := Transition{MessageType: form.List[1].Atom, Receiver: actor}
	var transitions []Transition
	for _, child := range form.List[2:] {
		switch child.head() {
		case "dwell_time_ms":
			value, err := parseLispFloat(path, child.List[1], "invalid dwell_time_ms")
			if err != nil {
				return nil, err
			}
			base.DwellTimeMS = &value
		case "latency_ms":
			return nil, lispErr(path, child, "latency_ms is not a handler property; use dwell_time_ms for actor processing time")
		case "bytes":
			return nil, lispErr(path, child, "bytes is derived from protobuf serialization; do not annotate bytes in convspec")
		case "queue":
			return nil, lispErr(path, child, "queue is not a handler property; every actor receives through its top-level inbox")
		case "when":
			transition, err := buildLispWhen(path, base, child)
			if err != nil {
				return nil, err
			}
			transitions = append(transitions, transition)
		default:
			return nil, lispErr(path, child, "unexpected transition form: "+child.head())
		}
	}
	if base.Receiver == "" {
		return nil, lispErr(path, form, "on form requires actor scope")
	}
	if len(transitions) == 0 {
		return nil, lispErr(path, form, "on form requires at least one when case")
	}
	return transitions, nil
}

func buildLispWhen(path string, base Transition, form lispNode) (Transition, error) {
	if len(form.List) < 4 {
		return Transition{}, lispErr(path, form, "expected: (when <condition> then <state> [(chance <p>|otherwise)] [(send <MessageType> ...)])")
	}
	guard, err := lispExprString(path, form.List[1])
	if err != nil {
		return Transition{}, err
	}
	if !form.List[2].isAtom() || form.List[2].Atom != "then" || !form.List[3].isAtom() {
		return Transition{}, lispErr(path, form, "expected: (when <condition> then <state> [(chance <p>|otherwise)] [(send <MessageType> ...)])")
	}
	transition := base
	transition.Guard = guard
	transition.Target = form.List[3].Atom
	transition.Chance = nil
	transition.Otherwise = false
	for _, child := range form.List[4:] {
		switch child.head() {
		case "chance":
			if len(child.List) != 2 {
				return Transition{}, lispErr(path, child, "expected: (chance <p>|otherwise)")
			}
			if err := parseLispChance(path, child, &transition, child.List[1]); err != nil {
				return Transition{}, err
			}
		case "send":
			sent, err := parseLispSend(path, child)
			if err != nil {
				return Transition{}, err
			}
			transition.Sends = append(transition.Sends, sent)
		default:
			return Transition{}, lispErr(path, child, "expected: (chance <p>|otherwise) or (send <MessageType> ...)")
		}
	}
	return transition, nil
}

func parseLispSend(path string, form lispNode) (SentMessage, error) {
	if len(form.List) < 2 || !form.List[1].isAtom() {
		return SentMessage{}, lispErr(path, form, "expected: (send <MessageType> [(set <field> <expr>) ...])")
	}
	sent := SentMessage{MessageType: form.List[1].Atom}
	for _, child := range form.List[2:] {
		if child.head() != "set" || len(child.List) != 3 || !child.List[1].isAtom() {
			return SentMessage{}, lispErr(path, child, "expected: (set <field> <expr>)")
		}
		value, err := lispExprString(path, child.List[2])
		if err != nil {
			return SentMessage{}, err
		}
		value = quoteLispStringArg(child.List[2], value)
		sent.Fields = append(sent.Fields, PayloadField{Name: child.List[1].Atom, Value: value})
	}
	return sent, nil
}

func parseLispChance(path string, form lispNode, transition *Transition, valueNode lispNode) error {
	if valueNode.isAtom() && valueNode.Atom == "otherwise" {
		transition.Chance = nil
		transition.Otherwise = true
		return nil
	}
	value, err := parseLispFloat(path, valueNode, "invalid chance value")
	if err != nil {
		return err
	}
	transition.Chance = &value
	transition.Otherwise = false
	return nil
}

func lispExprString(path string, node lispNode) (string, error) {
	if node.String {
		return node.Atom, nil
	}
	if node.isAtom() {
		return node.Atom, nil
	}
	if len(node.List) == 0 || !node.List[0].isAtom() {
		return "", lispErr(path, node, "expression must have an operator")
	}
	op := node.List[0].Atom
	args := node.List[1:]
	switch op {
	case "==", "!=", ">", ">=", "<", "<=", "->":
		if len(args) != 2 {
			return "", lispErr(path, node, op+" expects two operands")
		}
		left, err := lispExprString(path, args[0])
		if err != nil {
			return "", err
		}
		right, err := lispExprString(path, args[1])
		if err != nil {
			return "", err
		}
		return left + " " + op + " " + quoteLispStringArg(args[1], right), nil
	case "and", "or":
		if len(args) == 0 {
			return "", lispErr(path, node, op+" expects at least one operand")
		}
		parts, err := lispExprStrings(path, args)
		if err != nil {
			return "", err
		}
		return strings.Join(parts, " "+op+" "), nil
	case "not", "!":
		if len(args) != 1 {
			return "", lispErr(path, node, op+" expects one operand")
		}
		expr, err := lispExprString(path, args[0])
		if err != nil {
			return "", err
		}
		return "!(" + expr + ")", nil
	default:
		parts, err := lispExprStrings(path, args)
		if err != nil {
			return "", err
		}
		return op + "(" + strings.Join(parts, ", ") + ")", nil
	}
}

func lispExprStrings(path string, nodes []lispNode) ([]string, error) {
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		part, err := lispExprString(path, node)
		if err != nil {
			return nil, err
		}
		parts = append(parts, quoteLispStringArg(node, part))
	}
	return parts, nil
}

func quoteLispStringArg(node lispNode, value string) string {
	if node.String {
		return strconv.Quote(value)
	}
	return value
}

func parseLispFloat(path string, node lispNode, msg string) (float64, error) {
	if !node.isAtom() {
		return 0, lispErr(path, node, msg)
	}
	value, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(node.Atom, "/s"), "ms"), 64)
	if err != nil {
		return 0, lispErr(path, node, msg)
	}
	return value, nil
}

func parseLispInt(path string, node lispNode, msg string) (int, error) {
	if !node.isAtom() {
		return 0, lispErr(path, node, msg)
	}
	value, err := strconv.Atoi(node.Atom)
	if err != nil {
		return 0, lispErr(path, node, msg)
	}
	return value, nil
}

func (n lispNode) isAtom() bool {
	return len(n.List) == 0 && !n.String && n.Atom != ""
}

func (n lispNode) isListHead(head string) bool {
	return len(n.List) > 0 && n.List[0].isAtom() && n.List[0].Atom == head
}

func (n lispNode) head() string {
	if len(n.List) == 0 || !n.List[0].isAtom() {
		return ""
	}
	return n.List[0].Atom
}

func lispErr(path string, node lispNode, msg string) error {
	if node.Line > 0 {
		return ParseError{Path: path, Line: node.Line, Msg: msg}
	}
	return fmt.Errorf("%s: %s", path, msg)
}
