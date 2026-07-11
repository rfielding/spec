package convspec

import (
	"os"
	"regexp"
	"strconv"
)

var (
	lineCommentRE  = regexp.MustCompile(`//.*`)
	blockCommentRE = regexp.MustCompile(`(?s)/\*.*?\*/`)
	packageRE      = regexp.MustCompile(`\bpackage\s+([A-Za-z_][\w.]*)\s*;`)
	messageRE      = regexp.MustCompile(`(?s)\bmessage\s+([A-Za-z_]\w*)\s*\{(.*?)\}`)
	fieldRE        = regexp.MustCompile(`(?m)^\s*(?:optional\s+|repeated\s+)?([A-Za-z_][\w.]*)\s+([A-Za-z_]\w*)\s*=\s*(\d+)\s*(?:\[[^\]]*\])?\s*;`)
)

func ParseProtoFile(path string) (ProtoFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProtoFile{}, err
	}
	text := blockCommentRE.ReplaceAllString(string(data), "")
	text = lineCommentRE.ReplaceAllString(text, "")

	proto := ProtoFile{
		Path:     cleanPath(path),
		Messages: map[string]ProtoMessage{},
	}
	if match := packageRE.FindStringSubmatch(text); match != nil {
		proto.Package = match[1]
	}
	for _, match := range messageRE.FindAllStringSubmatch(text, -1) {
		name, body := match[1], match[2]
		message := ProtoMessage{Name: name, Fields: map[string]ProtoField{}}
		for _, fieldMatch := range fieldRE.FindAllStringSubmatch(body, -1) {
			num, err := strconv.Atoi(fieldMatch[3])
			if err != nil {
				return ProtoFile{}, err
			}
			field := ProtoField{Name: fieldMatch[2], Type: fieldMatch[1], Num: num}
			message.Fields[field.Name] = field
		}
		proto.Messages[name] = message
	}
	return proto, nil
}
