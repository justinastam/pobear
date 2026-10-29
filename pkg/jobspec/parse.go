package jobspec

import (
	"bytes"
	"io"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Decodo/pobear/pkg/utils"
)

type Mode int

const (
	ModePlain Mode = iota
	ModeRPC
)

type ValidationError = utils.ValidationError

func Parse(data []byte, mode Mode) (*Spec, error) {
	doc, verr := decodeSingleDocument(data)
	if verr != nil {
		return nil, verr
	}

	root, ok := doc.(map[string]any)
	if !ok {
		return nil, &ValidationError{Field: "$document", Message: "spec document must be a mapping"}
	}

	if err := utils.CheckUnknownKeys(root, "", "version", "source", "fetch", "limits", "stages", "sink"); err != nil {
		return nil, err
	}

	version, err := parseVersion(root)
	if err != nil {
		return nil, err
	}
	source, err := parseSource(root)
	if err != nil {
		return nil, err
	}
	fetch, err := parseFetch(root)
	if err != nil {
		return nil, err
	}
	limits, err := parseLimits(root)
	if err != nil {
		return nil, err
	}
	stages, err := parseStages(root, mode)
	if err != nil {
		return nil, err
	}
	sink, err := parseSink(root, mode)
	if err != nil {
		return nil, err
	}
	if err := checkSkipSeenSink(source, sink); err != nil {
		return nil, err
	}

	return &Spec{
		Version: version,
		Source:  source,
		Fetch:   fetch,
		Limits:  limits,
		Stages:  stages,
		Sink:    sink,
	}, nil
}

func decodeSingleDocument(data []byte) (any, *ValidationError) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var first any
	if err := dec.Decode(&first); err != nil {
		if err == io.EOF {
			return nil, &ValidationError{Field: "$document", Message: "spec document must contain exactly one YAML document"}
		}
		return nil, &ValidationError{Field: "$document", Message: "spec document is not valid YAML: " + err.Error()}
	}

	var second any
	err := dec.Decode(&second)
	if err == nil {
		return nil, &ValidationError{Field: "$document", Message: "spec document must contain exactly one YAML document"}
	}
	if err != io.EOF {
		return nil, &ValidationError{Field: "$document", Message: "spec document is not valid YAML: " + err.Error()}
	}

	return first, nil
}

func parseVersion(root map[string]any) (int, *ValidationError) {
	v, ok := root["version"]
	if !ok {
		return 0, utils.Errorf("version", "`version` is required")
	}
	n, ok := v.(int)
	if !ok || n != 1 {
		return 0, utils.Errorf("version", "`version` must be 1, got %v", v)
	}
	return n, nil
}

// checkSkipSeenSink rejects a spec whose sink would truncate the very file
// source.skipSeen resumes from: the seen set is read before the first write,
// so the only safe way to share one file is appending to it.
func checkSkipSeenSink(source Source, sink Sink) *ValidationError {
	if source.SkipSeen == "" || sink.Path == "" {
		return nil
	}
	if filepath.Clean(source.SkipSeen) != filepath.Clean(sink.Path) || sink.Append {
		return nil
	}
	return utils.Errorf("sink.append", "`sink.path` is the same file as `source.skipSeen`, which requires `sink.use: ndjson` with `sink.append: true` -- otherwise this run would overwrite the results it is resuming from")
}
