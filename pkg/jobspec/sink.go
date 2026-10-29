package jobspec

import "github.com/Decodo/pobear/pkg/utils"

func parseSink(root map[string]any, mode Mode) (Sink, *ValidationError) {
	raw, ok := root["sink"]
	if !ok {
		return Sink{Use: "stdout"}, nil
	}
	m, err := utils.AsMapping(raw, "sink", "use", "path", "append")
	if err != nil {
		return Sink{}, err
	}

	use := utils.GetString(m, "use", "stdout")
	if err := utils.EnumField("sink.use", use, "stdout", "ndjson", "json", "yaml", "stream"); err != nil {
		return Sink{}, err
	}
	if use == "stream" && mode != ModeRPC {
		return Sink{}, utils.Errorf("sink.use", "`sink.use: stream` is only allowed under `__rpc` mode")
	}

	pathRaw, hasPath := m["path"]
	var path string
	if hasPath {
		path, _ = pathRaw.(string)
	}

	switch use {
	case "ndjson", "json", "yaml":
		if !hasPath || path == "" {
			return Sink{}, utils.Errorf("sink.path", "`sink.path` is required when `sink.use` is `ndjson`, `json`, or `yaml`")
		}
	case "stdout", "stream":
		if hasPath {
			return Sink{}, utils.Errorf("sink.path", "`sink.path` is only allowed when `sink.use` is `ndjson`, `json`, or `yaml`")
		}
	}

	_, hasAppend := m["append"]
	appendMode, err := utils.GetBool(m, "append", "sink.append", false)
	if err != nil {
		return Sink{}, err
	}
	if hasAppend && use != "ndjson" {
		return Sink{}, utils.Errorf("sink.append", "`sink.append` is only allowed when `sink.use` is `ndjson`")
	}

	return Sink{Use: use, Path: path, Append: appendMode}, nil
}
