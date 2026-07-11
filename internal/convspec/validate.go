package convspec

import (
	"fmt"
	"strings"
)

func Validate(spec *Spec) error {
	participants := map[string]bool{}
	for _, participant := range spec.Participants {
		participants[participant] = true
	}
	var problems []string
	for _, conversation := range spec.Conversations {
		if _, ok := conversation.States[conversation.Start]; !ok {
			problems = append(problems, fmt.Sprintf("conversation %s: unknown start state %q", conversation.Name, conversation.Start))
		}
		for _, stateName := range conversation.Order {
			state := conversation.States[stateName]
			if state.Terminal != "" && len(state.Transitions) > 0 {
				problems = append(problems, fmt.Sprintf("conversation %s: terminal state %s has transitions", conversation.Name, state.Name))
			}
			for _, transition := range state.Transitions {
				if !participants[transition.Sender] {
					problems = append(problems, fmt.Sprintf("conversation %s.%s: unknown sender %s", conversation.Name, state.Name, transition.Sender))
				}
				if !participants[transition.Receiver] {
					problems = append(problems, fmt.Sprintf("conversation %s.%s: unknown receiver %s", conversation.Name, state.Name, transition.Receiver))
				}
				if !spec.messageIndex[transition.MessageType] {
					problems = append(problems, fmt.Sprintf("conversation %s.%s: unknown message %s", conversation.Name, state.Name, transition.MessageType))
				}
				if _, ok := conversation.States[transition.Target]; !ok {
					problems = append(problems, fmt.Sprintf("conversation %s.%s: unknown target state %q", conversation.Name, state.Name, transition.Target))
				}
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf(strings.Join(problems, "\n"))
	}
	return nil
}
