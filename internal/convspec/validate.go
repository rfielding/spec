package convspec

import (
	"fmt"
	"strings"
)

func Validate(spec *Spec) error {
	var problems []string
	participants := map[string]bool{}
	seenActors := map[string]bool{}
	for _, actor := range spec.Actors {
		if seenActors[actor.Name] {
			problems = append(problems, fmt.Sprintf("actor: duplicate actor %s", actor.Name))
		}
		seenActors[actor.Name] = true
		participants[actor.Name] = true
		if actor.Capacity <= 0 {
			problems = append(problems, fmt.Sprintf("actor: %s capacity must be greater than zero", actor.Name))
		}
	}
	seenReliability := map[string]bool{}
	for _, reliability := range spec.Reliability {
		if !participants[reliability.Actor] {
			problems = append(problems, fmt.Sprintf("reliability: unknown actor %s", reliability.Actor))
		}
		if seenReliability[reliability.Actor] {
			problems = append(problems, fmt.Sprintf("reliability: duplicate actor %s", reliability.Actor))
		}
		seenReliability[reliability.Actor] = true
		values := reliability.Parallel
		if len(values) == 0 {
			values = []float64{reliability.Availability}
		}
		for _, value := range values {
			if value <= 0 || value > 1 {
				problems = append(problems, fmt.Sprintf("reliability: %s availability %.6f must be > 0 and <= 1", reliability.Actor, value))
			}
		}
	}
	seenConversations := map[string]bool{}
	for _, conversation := range spec.Conversations {
		if seenConversations[conversation.Name] {
			problems = append(problems, fmt.Sprintf("conversation %s: duplicate conversation name", conversation.Name))
		}
		seenConversations[conversation.Name] = true
		if _, ok := conversation.States[conversation.Start]; !ok {
			problems = append(problems, fmt.Sprintf("conversation %s: unknown start state %q", conversation.Name, conversation.Start))
		}
		if conversation.StartActor != "" && !participants[conversation.StartActor] {
			problems = append(problems, fmt.Sprintf("conversation %s: unknown start actor %s", conversation.Name, conversation.StartActor))
		}
		if conversation.StartMessage != "" && !spec.messageIndex[conversation.StartMessage] {
			problems = append(problems, fmt.Sprintf("conversation %s: unknown start message %s", conversation.Name, conversation.StartMessage))
		}
		for _, stateName := range conversation.Order {
			state := conversation.States[stateName]
			if state.Terminal != "" && len(state.Transitions) > 0 {
				problems = append(problems, fmt.Sprintf("conversation %s: terminal state %s has transitions", conversation.Name, state.Name))
			}
			for _, transition := range state.Transitions {
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
		for _, metric := range conversation.Metrics {
			if metric.Message != "" && !spec.messageIndex[metric.Message] {
				problems = append(problems, fmt.Sprintf("conversation %s metric %s: unknown message %s", conversation.Name, metric.Name, metric.Message))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf(strings.Join(problems, "\n"))
	}
	return nil
}
