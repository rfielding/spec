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
	Name        string  `json:"name"`
	Outcome     string  `json:"outcome"`
	Probability float64 `json:"probability"`
	LatencyMS   float64 `json:"latency_ms"`
	Bytes       float64 `json:"bytes"`
}

type QueueMetric struct {
	Name            string  `json:"name"`
	ArrivalRate     float64 `json:"arrival_rate_per_s"`
	ServiceTimeMS   float64 `json:"service_time_ms"`
	Workers         int     `json:"workers"`
	Utilization     float64 `json:"utilization"`
	ExpectedQueue   float64 `json:"expected_queue_length"`
	ExpectedWaitMS  float64 `json:"expected_wait_ms"`
	ExpectedTotalMS float64 `json:"expected_total_ms"`
	Status          string  `json:"status"`
}

func ComputeMetrics(spec *Spec) MetricsReport {
	report := MetricsReport{}
	for _, conversation := range spec.Conversations {
		report.Conversations = append(report.Conversations, computeConversationMetrics(conversation))
	}
	return report
}

func computeConversationMetrics(conversation Conversation) ConversationMetrics {
	metrics := ConversationMetrics{Name: conversation.DiagramName()}
	paths := enumeratePaths(conversation)
	outcomes := map[string]float64{}
	for i, path := range paths {
		scenario := ScenarioMetric{
			Name:        pathTitle(conversation, i+1, path),
			Outcome:     terminalForPath(conversation, path),
			Probability: 1,
		}
		for _, step := range path {
			transition := step.Transition
			if transition.Chance != nil {
				scenario.Probability *= *transition.Chance
				metrics.HasQuantities = true
			}
			if transition.LatencyMS != nil {
				scenario.LatencyMS += *transition.LatencyMS
				metrics.HasQuantities = true
			}
			if transition.Bytes != nil {
				scenario.Bytes += *transition.Bytes
				metrics.HasQuantities = true
			}
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
		if count > 0 && math.Abs(sum-1.0) > 0.001 {
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

func terminalForPath(conversation Conversation, path []pathStep) string {
	if len(path) == 0 {
		return conversation.Start
	}
	return path[len(path)-1].Transition.Target
}

func computeQueueMetric(queue QueueSpec) QueueMetric {
	serviceRate := 1000.0 / queue.ServiceTimeMS
	c := float64(queue.Workers)
	rho := queue.ArrivalRate / (c * serviceRate)
	metric := QueueMetric{
		Name:          queue.Name,
		ArrivalRate:   queue.ArrivalRate,
		ServiceTimeMS: queue.ServiceTimeMS,
		Workers:       queue.Workers,
		Utilization:   rho,
		Status:        "stable",
	}
	if rho >= 1 {
		metric.Status = "saturated"
		metric.ExpectedQueue = math.Inf(1)
		metric.ExpectedWaitMS = math.Inf(1)
		metric.ExpectedTotalMS = math.Inf(1)
		return metric
	}
	lq := erlangC(queue.ArrivalRate, serviceRate, queue.Workers) * rho / (1 - rho)
	wqSeconds := lq / queue.ArrivalRate
	metric.ExpectedQueue = lq
	metric.ExpectedWaitMS = wqSeconds * 1000
	metric.ExpectedTotalMS = metric.ExpectedWaitMS + queue.ServiceTimeMS
	if rho >= 0.85 {
		metric.Status = "near_saturation"
	}
	return metric
}

func erlangC(lambda float64, mu float64, workers int) float64 {
	c := float64(workers)
	a := lambda / mu
	rho := a / c
	sum := 0.0
	for n := 0; n < workers; n++ {
		sum += math.Pow(a, float64(n)) / factorial(n)
	}
	top := math.Pow(a, c) / (factorial(workers) * (1 - rho))
	return top / (sum + top)
}

func factorial(n int) float64 {
	out := 1.0
	for i := 2; i <= n; i++ {
		out *= float64(i)
	}
	return out
}
