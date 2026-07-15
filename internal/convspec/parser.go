package convspec

import (
	"fmt"
	"os"
	"path/filepath"
)

type ParseError struct {
	Path string
	Line int
	Msg  string
}

func (e ParseError) Error() string {
	if e.Line <= 0 {
		return fmt.Sprintf("%s: %s", e.Path, e.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Msg)
}

func ParseFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	spec, err := parseLispSpec(path, data)
	if err != nil {
		return nil, err
	}
	return finishParsedSpec(spec, path)
}

func finishParsedSpec(spec *Spec, path string) (*Spec, error) {
	baseDir := filepath.Dir(path)
	for _, importPath := range spec.Imports {
		protoPath := filepath.Join(baseDir, importPath)
		protoFile, err := ParseProtoFile(protoPath)
		if err != nil {
			return nil, fmt.Errorf("%s: imported proto %q: %w", path, importPath, err)
		}
		spec.ProtoFiles = append(spec.ProtoFiles, protoFile)
	}
	spec.buildMessageIndex()
	if err := Validate(spec); err != nil {
		return nil, err
	}
	return spec, nil
}
