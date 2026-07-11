package convspec

import "fmt"

type AnalysisReport struct {
	Conversations []ConversationAnalysis `json:"conversations"`
	Warnings      []string               `json:"warnings,omitempty"`
}

type ConversationAnalysis struct {
	Name              string   `json:"name"`
	Start             string   `json:"start"`
	StateCount        int      `json:"state_count"`
	ReachableStates   int      `json:"reachable_states"`
	TransitionCount   int      `json:"transition_count"`
	TerminalPathCount int      `json:"terminal_path_count"`
	AcceptStates      []string `json:"accept_states,omitempty"`
	RejectStates      []string `json:"reject_states,omitempty"`
}

func Analyze(spec *Spec) AnalysisReport {
	var report AnalysisReport
	for _, conversation := range spec.Conversations {
		reachable := reachableStates(conversation)
		analysis := ConversationAnalysis{
			Name:              conversation.DiagramName(),
			Start:             conversation.Start,
			StateCount:        len(conversation.States),
			ReachableStates:   len(reachable),
			TerminalPathCount: len(enumeratePaths(conversation)),
		}
		for _, stateName := range conversation.Order {
			state := conversation.States[stateName]
			analysis.TransitionCount += len(state.Transitions)
			if !reachable[stateName] {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s: state %s is unreachable", conversation.DiagramName(), stateName))
			}
			switch state.Terminal {
			case "accept":
				analysis.AcceptStates = append(analysis.AcceptStates, stateName)
			case "reject":
				analysis.RejectStates = append(analysis.RejectStates, stateName)
			case "":
				if len(state.Transitions) == 0 {
					report.Warnings = append(report.Warnings, fmt.Sprintf("%s: state %s is non-terminal with no outgoing transitions", conversation.DiagramName(), stateName))
				}
			}
		}
		report.Conversations = append(report.Conversations, analysis)
	}
	return report
}

func reachableStates(conversation Conversation) map[string]bool {
	reachable := map[string]bool{}
	var walk func(string)
	walk = func(stateName string) {
		if reachable[stateName] {
			return
		}
		state, ok := conversation.States[stateName]
		if !ok {
			return
		}
		reachable[stateName] = true
		for _, transition := range state.Transitions {
			walk(transition.Target)
		}
	}
	walk(conversation.Start)
	return reachable
}
