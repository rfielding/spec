package convspec

import (
	"fmt"
	"strings"
	"unicode"
)

type AssertionResult struct {
	Conversation string   `json:"conversation"`
	Name         string   `json:"name"`
	Formula      string   `json:"formula"`
	Pass         bool     `json:"pass"`
	Satisfying   []string `json:"satisfying_states,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func EvaluateAssertions(spec *Spec) []AssertionResult {
	var results []AssertionResult
	for _, conversation := range spec.Conversations {
		graph := newCTLGraph(conversation)
		for _, assertion := range conversation.Asserts {
			result := AssertionResult{
				Conversation: conversation.DiagramName(),
				Name:         assertion.Name,
				Formula:      assertion.Formula,
			}
			expr, err := parseCTL(assertion.Formula)
			if err != nil {
				result.Error = err.Error()
				results = append(results, result)
				continue
			}
			states := evalCTL(expr, graph)
			result.Satisfying = sortedStateSubset(conversation.Order, states)
			result.Pass = states[conversation.Start]
			results = append(results, result)
		}
	}
	return results
}

type ctlGraph struct {
	conversation Conversation
	successors   map[string][]string
	predecessors map[string][]string
	props        map[string]map[string]bool
}

func newCTLGraph(conversation Conversation) ctlGraph {
	graph := ctlGraph{
		conversation: conversation,
		successors:   map[string][]string{},
		predecessors: map[string][]string{},
		props:        map[string]map[string]bool{},
	}
	for _, stateName := range conversation.Order {
		state := conversation.States[stateName]
		props := map[string]bool{
			state.Name: true,
		}
		if state.Terminal != "" {
			props[state.Terminal] = true
			props["terminal"] = true
		}
		for _, emission := range state.Emits {
			props[emission] = true
		}
		graph.props[stateName] = props
		if len(state.Transitions) == 0 {
			graph.successors[stateName] = []string{stateName}
			graph.predecessors[stateName] = append(graph.predecessors[stateName], stateName)
			continue
		}
		for _, transition := range state.Transitions {
			graph.successors[stateName] = append(graph.successors[stateName], transition.Target)
			graph.predecessors[transition.Target] = append(graph.predecessors[transition.Target], stateName)
		}
	}
	return graph
}

type ctlKind int

const (
	ctlAtom ctlKind = iota
	ctlTrue
	ctlFalse
	ctlNot
	ctlAnd
	ctlOr
	ctlImplies
	ctlEF
	ctlAF
	ctlEG
	ctlAG
)

type ctlExpr struct {
	kind  ctlKind
	value string
	left  *ctlExpr
	right *ctlExpr
}

func evalCTL(expr *ctlExpr, graph ctlGraph) map[string]bool {
	switch expr.kind {
	case ctlAtom:
		out := map[string]bool{}
		for _, stateName := range graph.conversation.Order {
			out[stateName] = graph.props[stateName][expr.value]
		}
		return out
	case ctlTrue:
		out := map[string]bool{}
		for _, stateName := range graph.conversation.Order {
			out[stateName] = true
		}
		return out
	case ctlFalse:
		return map[string]bool{}
	case ctlNot:
		return setNot(evalCTL(expr.left, graph), graph)
	case ctlAnd:
		return setAnd(evalCTL(expr.left, graph), evalCTL(expr.right, graph), graph)
	case ctlOr:
		return setOr(evalCTL(expr.left, graph), evalCTL(expr.right, graph), graph)
	case ctlImplies:
		return setOr(setNot(evalCTL(expr.left, graph), graph), evalCTL(expr.right, graph), graph)
	case ctlEF:
		return ctlEFix(evalCTL(expr.left, graph), graph)
	case ctlAF:
		return ctlAFix(evalCTL(expr.left, graph), graph)
	case ctlEG:
		return ctlEGFix(evalCTL(expr.left, graph), graph)
	case ctlAG:
		return setNot(ctlEFix(setNot(evalCTL(expr.left, graph), graph), graph), graph)
	default:
		return map[string]bool{}
	}
}

func ctlEFix(seed map[string]bool, graph ctlGraph) map[string]bool {
	result := copySet(seed)
	changed := true
	for changed {
		changed = false
		for state := range result {
			for _, pred := range graph.predecessors[state] {
				if !result[pred] {
					result[pred] = true
					changed = true
				}
			}
		}
	}
	return result
}

func ctlAFix(seed map[string]bool, graph ctlGraph) map[string]bool {
	result := copySet(seed)
	changed := true
	for changed {
		changed = false
		for _, stateName := range graph.conversation.Order {
			if result[stateName] {
				continue
			}
			successors := graph.successors[stateName]
			if len(successors) == 0 {
				continue
			}
			all := true
			for _, successor := range successors {
				if !result[successor] {
					all = false
					break
				}
			}
			if all {
				result[stateName] = true
				changed = true
			}
		}
	}
	return result
}

func ctlEGFix(seed map[string]bool, graph ctlGraph) map[string]bool {
	result := copySet(seed)
	changed := true
	for changed {
		changed = false
		for state := range result {
			hasSuccessor := false
			for _, successor := range graph.successors[state] {
				if result[successor] {
					hasSuccessor = true
					break
				}
			}
			if !hasSuccessor {
				delete(result, state)
				changed = true
			}
		}
	}
	return result
}

func setNot(in map[string]bool, graph ctlGraph) map[string]bool {
	out := map[string]bool{}
	for _, stateName := range graph.conversation.Order {
		out[stateName] = !in[stateName]
	}
	return out
}

func setAnd(left map[string]bool, right map[string]bool, graph ctlGraph) map[string]bool {
	out := map[string]bool{}
	for _, stateName := range graph.conversation.Order {
		out[stateName] = left[stateName] && right[stateName]
	}
	return out
}

func setOr(left map[string]bool, right map[string]bool, graph ctlGraph) map[string]bool {
	out := map[string]bool{}
	for _, stateName := range graph.conversation.Order {
		out[stateName] = left[stateName] || right[stateName]
	}
	return out
}

func copySet(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range in {
		if value {
			out[key] = true
		}
	}
	return out
}

func sortedStateSubset(order []string, set map[string]bool) []string {
	var states []string
	for _, stateName := range order {
		if set[stateName] {
			states = append(states, stateName)
		}
	}
	return states
}

type ctlParser struct {
	tokens []string
	pos    int
}

func parseCTL(input string) (*ctlExpr, error) {
	parser := ctlParser{tokens: tokenizeCTL(input)}
	if len(parser.tokens) == 0 {
		return nil, fmt.Errorf("empty CTL formula")
	}
	expr, err := parser.parseImplies()
	if err != nil {
		return nil, err
	}
	if parser.peek() != "" {
		return nil, fmt.Errorf("unexpected token %q", parser.peek())
	}
	return expr, nil
}

func (p *ctlParser) parseImplies() (*ctlExpr, error) {
	left, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek() == "->" {
		p.pop()
		right, err := p.parseImplies()
		if err != nil {
			return nil, err
		}
		return &ctlExpr{kind: ctlImplies, left: left, right: right}, nil
	}
	return left, nil
}

func (p *ctlParser) parseOr() (*ctlExpr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "or" {
		p.pop()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &ctlExpr{kind: ctlOr, left: left, right: right}
	}
	return left, nil
}

func (p *ctlParser) parseAnd() (*ctlExpr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek() == "and" {
		p.pop()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &ctlExpr{kind: ctlAnd, left: left, right: right}
	}
	return left, nil
}

func (p *ctlParser) parseUnary() (*ctlExpr, error) {
	token := p.peek()
	switch token {
	case "not", "!":
		p.pop()
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ctlExpr{kind: ctlNot, left: expr}, nil
	case "EF", "AF", "EG", "AG", "possibly", "risks", "eventually", "becomes", "always", "possibly_always", "can_stabilize", "can_become_stable":
		p.pop()
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if token == "can_stabilize" || token == "can_become_stable" {
			return &ctlExpr{kind: ctlEF, left: &ctlExpr{kind: ctlEG, left: expr}}, nil
		}
		return &ctlExpr{kind: ctlKindFor(token), left: expr}, nil
	case "(":
		p.pop()
		expr, err := p.parseImplies()
		if err != nil {
			return nil, err
		}
		if p.pop() != ")" {
			return nil, fmt.Errorf("missing closing )")
		}
		return expr, nil
	case "":
		return nil, fmt.Errorf("unexpected end of formula")
	default:
		p.pop()
		switch token {
		case "true":
			return &ctlExpr{kind: ctlTrue}, nil
		case "false":
			return &ctlExpr{kind: ctlFalse}, nil
		default:
			return &ctlExpr{kind: ctlAtom, value: token}, nil
		}
	}
}

func ctlKindFor(token string) ctlKind {
	switch token {
	case "EF":
		return ctlEF
	case "AF":
		return ctlAF
	case "EG":
		return ctlEG
	case "AG":
		return ctlAG
	case "possibly":
		return ctlEF
	case "risks":
		return ctlEF
	case "eventually":
		return ctlAF
	case "becomes":
		return ctlAF
	case "possibly_always":
		return ctlEG
	case "always":
		return ctlAG
	default:
		return ctlAtom
	}
}

func (p *ctlParser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *ctlParser) pop() string {
	token := p.peek()
	if token != "" {
		p.pos++
	}
	return token
}

func tokenizeCTL(input string) []string {
	var tokens []string
	for i := 0; i < len(input); {
		r := rune(input[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if strings.HasPrefix(input[i:], "->") {
			tokens = append(tokens, "->")
			i += 2
			continue
		}
		if strings.ContainsRune("()!", r) {
			tokens = append(tokens, string(r))
			i++
			continue
		}
		start := i
		for i < len(input) {
			r = rune(input[i])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' {
				i++
				continue
			}
			break
		}
		if start == i {
			tokens = append(tokens, string(input[i]))
			i++
			continue
		}
		tokens = append(tokens, input[start:i])
	}
	return tokens
}
