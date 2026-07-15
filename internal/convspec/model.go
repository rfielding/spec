package convspec

import "path/filepath"

type ProtoField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Num  int    `json:"number"`
}

type ProtoMessage struct {
	Name   string                `json:"name"`
	Fields map[string]ProtoField `json:"fields"`
}

type ProtoFile struct {
	Path     string                  `json:"path"`
	Package  string                  `json:"package,omitempty"`
	Messages map[string]ProtoMessage `json:"messages"`
}

type Transition struct {
	Sender      string   `json:"sender"`
	Receiver    string   `json:"receiver"`
	MessageType string   `json:"message_type"`
	Bind        string   `json:"bind,omitempty"`
	Guards      []string `json:"guards,omitempty"`
	Target      string   `json:"target"`
	Chance      *float64 `json:"chance,omitempty"`
	Otherwise   bool     `json:"otherwise,omitempty"`
	DwellTimeMS *float64 `json:"dwell_time_ms,omitempty"`
	LatencyMS   *float64 `json:"latency_ms,omitempty"`
	Bytes       *float64 `json:"bytes,omitempty"`
	Queue       string   `json:"queue,omitempty"`
}

type State struct {
	Name        string       `json:"name"`
	Actor       string       `json:"actor,omitempty"`
	Terminal    string       `json:"terminal,omitempty"`
	Emits       []string     `json:"emits,omitempty"`
	Transitions []Transition `json:"transitions,omitempty"`
}

type Conversation struct {
	Name    string           `json:"name"`
	Version string           `json:"version,omitempty"`
	Start   string           `json:"start"`
	States  map[string]State `json:"states"`
	Order   []string         `json:"-"`
	Asserts []Assertion      `json:"assertions,omitempty"`
	Queues  []QueueSpec      `json:"queues,omitempty"`
}

func (c Conversation) DiagramName() string {
	if c.Version == "" {
		return c.Name
	}
	return c.Name + "_v" + c.Version
}

type Spec struct {
	Name          string            `json:"spec"`
	SourcePath    string            `json:"source_path"`
	Imports       []string          `json:"imports,omitempty"`
	Participants  []string          `json:"participants,omitempty"`
	Reliability   []ReliabilitySpec `json:"reliability,omitempty"`
	Conversations []Conversation    `json:"conversations,omitempty"`
	ProtoFiles    []ProtoFile       `json:"proto_files,omitempty"`
	Messages      []ProtoMessage    `json:"messages,omitempty"`
	messageIndex  map[string]bool   `json:"-"`
}

type Assertion struct {
	Name    string `json:"name"`
	Formula string `json:"formula"`
}

type QueueSpec struct {
	Name          string  `json:"name"`
	Actor         string  `json:"actor,omitempty"`
	Kind          string  `json:"kind,omitempty"`
	ArrivalRate   float64 `json:"arrival_rate_per_s"`
	ServiceTimeMS float64 `json:"service_time_ms"`
	Capacity      int     `json:"capacity,omitempty"`
}

type ReliabilitySpec struct {
	Actor        string    `json:"actor"`
	Availability float64   `json:"availability,omitempty"`
	Parallel     []float64 `json:"parallel,omitempty"`
}

func (s *Spec) buildMessageIndex() {
	s.messageIndex = map[string]bool{}
	s.Messages = nil
	for _, protoFile := range s.ProtoFiles {
		for _, message := range protoFile.Messages {
			s.messageIndex[message.Name] = true
			s.Messages = append(s.Messages, message)
		}
	}
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}
