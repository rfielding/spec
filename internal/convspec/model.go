package convspec

import (
	"path/filepath"
	"sort"
)

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
	Receiver    string        `json:"receiver,omitempty"`
	MessageType string        `json:"message_type"`
	Guard       string        `json:"guard,omitempty"`
	Target      string        `json:"target"`
	Chance      *float64      `json:"chance,omitempty"`
	Otherwise   bool          `json:"otherwise,omitempty"`
	DwellTimeMS *float64      `json:"dwell_time_ms,omitempty"`
	Sends       []SentMessage `json:"sends,omitempty"`
}

type SentMessage struct {
	MessageType string         `json:"message_type"`
	Fields      []PayloadField `json:"fields,omitempty"`
}

type PayloadField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type State struct {
	Name        string       `json:"name"`
	Actor       string       `json:"actor,omitempty"`
	Terminal    string       `json:"terminal,omitempty"`
	StateIs     []string     `json:"state_is,omitempty"`
	Transitions []Transition `json:"transitions,omitempty"`
}

type Conversation struct {
	Name         string           `json:"name"`
	Version      string           `json:"version,omitempty"`
	Start        string           `json:"start"`
	StartActor   string           `json:"start_actor,omitempty"`
	StartMessage string           `json:"start_message,omitempty"`
	States       map[string]State `json:"states"`
	Order        []string         `json:"-"`
	Asserts      []Assertion      `json:"assertions,omitempty"`
	Metrics      []MetricSpec     `json:"metrics,omitempty"`
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
	Includes      []string          `json:"includes,omitempty"`
	Actors        []ActorSpec       `json:"actors,omitempty"`
	Reliability   []ReliabilitySpec `json:"reliability,omitempty"`
	Asserts       []Assertion       `json:"assertions,omitempty"`
	Conversations []Conversation    `json:"conversations,omitempty"`
	ProtoFiles    []ProtoFile       `json:"proto_files,omitempty"`
	Messages      []ProtoMessage    `json:"messages,omitempty"`
	messageIndex  map[string]bool   `json:"-"`
}

type Assertion struct {
	Name    string `json:"name"`
	Formula string `json:"formula"`
}

type ActorSpec struct {
	Name     string       `json:"name"`
	Role     string       `json:"role,omitempty"`
	Capacity int          `json:"capacity,omitempty"`
	Params   []ActorParam `json:"params,omitempty"`
}

type ActorParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ReliabilitySpec struct {
	Actor        string    `json:"actor"`
	Availability float64   `json:"availability,omitempty"`
	Parallel     []float64 `json:"parallel,omitempty"`
}

type MetricSpec struct {
	Name    string `json:"name"`
	Chart   string `json:"chart"`
	Message string `json:"message,omitempty"`
	Value   string `json:"value,omitempty"`
	GroupBy string `json:"group_by,omitempty"`
	Window  string `json:"window,omitempty"`
	Reducer string `json:"reducer,omitempty"`
}

func (s *Spec) buildMessageIndex() {
	s.messageIndex = map[string]bool{}
	s.Messages = nil
	for _, protoFile := range s.ProtoFiles {
		names := make([]string, 0, len(protoFile.Messages))
		for name := range protoFile.Messages {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			message := protoFile.Messages[name]
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
