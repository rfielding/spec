package convspec

import (
	"fmt"
	"math"
)

type MetricsReport struct {
	Conversations []ConversationMetrics `json:"conversations,omitempty"`
}

type ConversationMetrics struct {
	Name          string           `json:"name"`
	Outcomes      []OutcomeMetric  `json:"outcomes,omitempty"`
	Scenarios     []ScenarioMetric `json:"scenarios,omitempty"`
	Queues        []QueueMetric    `json:"queues,omitempty"`
	Warnings      []string         `json:"warnings,omitempty"`
	HasQuantities bool             `json:"has_quantities"`
}

type OutcomeMetric struct {
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

type ScenarioMetric struct {
	Name         string              `json:"name"`
	Outcome      string              `json:"outcome"`
	Probability  float64             `json:"probability"`
	LatencyMS    float64             `json:"latency_ms"`
	Bytes        float64             `json:"bytes"`
	Availability float64             `json:"availability,omitempty"`
	Reliability  []ReliabilityMetric `json:"reliability,omitempty"`
	ByteFlows    []ByteFlowMetric    `json:"byte_flows,omitempty"`
}

type ByteFlowMetric struct {
	From  string  `json:"from"`
	To    string  `json:"to"`
	Bytes float64 `json:"bytes"`
}

type ReliabilityMetric struct {
	Actor        string    `json:"actor"`
	Availability float64   `json:"availability"`
	Parallel     []float64 `json:"parallel,omitempty"`
}

type QueueMetric struct {
	Name            string  `json:"name"`
	ArrivalRate     float64 `json:"arrival_rate_per_s,omitempty"`
	ServiceTimeMS   float64 `json:"service_time_ms,omitempty"`
	Capacity        int     `json:"capacity"`
	OfferedLoad     float64 `json:"offered_load,omitempty"`
	ExpectedQueue   float64 `json:"expected_queue_length,omitempty"`
	ExpectedWaitMS  float64 `json:"expected_wait_ms,omitempty"`
	FullProbability float64 `json:"full_probability,omitempty"`
	BlocksWhenFull  bool    `json:"blocks_when_full"`
	Status          string  `json:"status"`
}

func ComputeMetrics(spec *Spec) MetricsReport {
	report := MetricsReport{}
	for _, conversation := range spec.Conversations {
		report.Conversations = append(report.Conversations, computeConversationMetrics(spec, conversation))
	}
	return report
}

func computeConversationMetrics(spec *Spec, conversation Conversation) ConversationMetrics {
	metrics := ConversationMetrics{Name: conversation.DiagramName()}
	paths := enumeratePaths(conversation)
	outcomes := map[string]float64{}
	reliabilityByActor := reliabilityIndex(spec)
	for i, path := range paths {
		scenario := ScenarioMetric{
			Name:        pathTitle(conversation, i+1, path),
			Outcome:     terminalForPath(conversation, path),
			Probability: 1,
		}
		byteFlows := map[string]*ByteFlowMetric{}
		var byteFlowOrder []string
		for _, step := range path {
			transition := step.Transition
			if chance := transitionChance(conversation, step.State, transition); chance != nil {
				scenario.Probability *= *chance
				metrics.HasQuantities = true
			}
			if transitionDwellMS(transition) != nil {
				scenario.LatencyMS += *transitionDwellMS(transition)
				metrics.HasQuantities = true
			}
			messageBytes := estimatedTransitionBytes(spec, transition)
			if messageBytes > 0 {
				scenario.Bytes += messageBytes
				key := transition.Sender + "\x00" + transition.Receiver
				flow := byteFlows[key]
				if flow == nil {
					flow = &ByteFlowMetric{From: transition.Sender, To: transition.Receiver}
					byteFlows[key] = flow
					byteFlowOrder = append(byteFlowOrder, key)
				}
				flow.Bytes += messageBytes
				metrics.HasQuantities = true
			}
		}
		for _, key := range byteFlowOrder {
			scenario.ByteFlows = append(scenario.ByteFlows, *byteFlows[key])
		}
		if len(reliabilityByActor) > 0 {
			scenario.Availability = 1
			for _, actor := range actorsForPath(path) {
				reliability, ok := reliabilityByActor[actor]
				if !ok {
					metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("scenario %s uses actor %s without reliability annotation; assuming 1.0", scenario.Name, actor))
					reliability = ReliabilityMetric{Actor: actor, Availability: 1}
				}
				scenario.Reliability = append(scenario.Reliability, reliability)
				scenario.Availability *= reliability.Availability
			}
			metrics.HasQuantities = true
		}
		outcomes[scenario.Outcome] += scenario.Probability
		metrics.Scenarios = append(metrics.Scenarios, scenario)
	}
	for _, stateName := range conversation.Order {
		sum := 0.0
		count := 0
		for _, transition := range conversation.States[stateName].Transitions {
			if transition.Chance != nil {
				sum += *transition.Chance
				count++
			}
		}
		if hasOtherwise(conversation.States[stateName].Transitions) {
			count++
			if sum > 1.001 {
				metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("%s outgoing explicit chance sum is %.3f before chance otherwise", stateName, sum))
			}
		} else if count > 0 && math.Abs(sum-1.0) > 0.001 {
			metrics.Warnings = append(metrics.Warnings, fmt.Sprintf("%s outgoing chance sum is %.3f, not 1.0", stateName, sum))
		}
	}
	for outcome, probability := range outcomes {
		metrics.Outcomes = append(metrics.Outcomes, OutcomeMetric{Name: outcome, Probability: probability})
	}
	for _, queue := range conversation.Queues {
		metrics.HasQuantities = true
		metrics.Queues = append(metrics.Queues, computeQueueMetric(queue))
	}
	return metrics
}

func transitionChance(conversation Conversation, stateName string, transition Transition) *float64 {
	if transition.Chance != nil {
		return transition.Chance
	}
	if !transition.Otherwise {
		return nil
	}
	remainder := 1 - explicitChanceSum(conversation.States[stateName].Transitions)
	if remainder < 0 {
		remainder = 0
	}
	return &remainder
}

func explicitChanceSum(transitions []Transition) float64 {
	sum := 0.0
	for _, transition := range transitions {
		if transition.Chance != nil {
			sum += *transition.Chance
		}
	}
	return sum
}

func hasOtherwise(transitions []Transition) bool {
	for _, transition := range transitions {
		if transition.Otherwise {
			return true
		}
	}
	return false
}

func transitionDwellMS(transition Transition) *float64 {
	if transition.DwellTimeMS != nil {
		return transition.DwellTimeMS
	}
	return transition.LatencyMS
}

func estimatedTransitionBytes(spec *Spec, transition Transition) float64 {
	if transition.Bytes != nil {
		return *transition.Bytes
	}
	for _, message := range spec.Messages {
		if message.Name == transition.MessageType {
			return estimateProtoMessageBytes(message)
		}
	}
	return 0
}

func estimateProtoMessageBytes(message ProtoMessage) float64 {
	total := 0
	for _, field := range message.Fields {
		total += estimateProtoFieldBytes(field)
	}
	return float64(total)
}

func estimateProtoFieldBytes(field ProtoField) int {
	tagBytes := protoVarintBytes(uint64(field.Num << 3))
	switch field.Type {
	case "double", "fixed64", "sfixed64":
		return tagBytes + 8
	case "float", "fixed32", "sfixed32":
		return tagBytes + 4
	case "bool":
		return tagBytes + 1
	case "string", "bytes":
		nominalLen := 16
		return tagBytes + protoVarintBytes(uint64(nominalLen)) + nominalLen
	case "int32", "uint32", "sint32", "enum":
		return tagBytes + 5
	case "int64", "uint64", "sint64":
		return tagBytes + 10
	default:
		nominalLen := 16
		return tagBytes + protoVarintBytes(uint64(nominalLen)) + nominalLen
	}
}

func protoVarintBytes(value uint64) int {
	count := 1
	for value >= 0x80 {
		value >>= 7
		count++
	}
	return count
}

func reliabilityIndex(spec *Spec) map[string]ReliabilityMetric {
	if len(spec.Reliability) == 0 {
		return nil
	}
	out := map[string]ReliabilityMetric{}
	for _, item := range spec.Reliability {
		metric := ReliabilityMetric{Actor: item.Actor, Availability: item.Availability}
		if len(item.Parallel) > 0 {
			metric.Parallel = append(metric.Parallel, item.Parallel...)
			down := 1.0
			for _, availability := range item.Parallel {
				down *= 1 - availability
			}
			metric.Availability = 1 - down
		}
		out[item.Actor] = metric
	}
	return out
}

func actorsForPath(path []pathStep) []string {
	seen := map[string]bool{}
	var actors []string
	for _, step := range path {
		for _, actor := range []string{step.Transition.Sender, step.Transition.Receiver} {
			if !seen[actor] {
				seen[actor] = true
				actors = append(actors, actor)
			}
		}
	}
	return actors
}

func terminalForPath(conversation Conversation, path []pathStep) string {
	if len(path) == 0 {
		return conversation.Start
	}
	return path[len(path)-1].Transition.Target
}

func computeQueueMetric(queue QueueSpec) QueueMetric {
	metric := QueueMetric{
		Name:           queue.Name,
		ArrivalRate:    queue.ArrivalRate,
		ServiceTimeMS:  queue.ServiceTimeMS,
		Capacity:       queue.Capacity,
		BlocksWhenFull: true,
		Status:         "capacity_only",
	}
	if queue.ArrivalRate <= 0 || queue.ServiceTimeMS <= 0 {
		return metric
	}
	rho := queue.ArrivalRate * queue.ServiceTimeMS / 1000
	metric.OfferedLoad = rho
	if rho >= 1 {
		metric.Status = "blocking"
		metric.ExpectedQueue = math.Inf(1)
		metric.ExpectedWaitMS = math.Inf(1)
		metric.FullProbability = 1
		return metric
	}
	lq := rho * rho / (1 - rho)
	wqSeconds := lq / queue.ArrivalRate
	metric.ExpectedQueue = lq
	metric.ExpectedWaitMS = wqSeconds * 1000
	metric.FullProbability = finiteQueueFullProbability(rho, queue.Capacity)
	metric.Status = "draining"
	if metric.FullProbability > 0.01 {
		metric.Status = "blocking_risk"
	}
	return metric
}

func finiteQueueFullProbability(rho float64, capacity int) float64 {
	if capacity <= 0 {
		return 0
	}
	if math.Abs(rho-1) < 0.000001 {
		return 1 / float64(capacity+1)
	}
	top := (1 - rho) * math.Pow(rho, float64(capacity))
	bottom := 1 - math.Pow(rho, float64(capacity+1))
	return top / bottom
}
