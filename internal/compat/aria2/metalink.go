package aria2

import (
	"context"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/protocol/metalink"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// parseAddMetalinkParams 解析 aria2.addMetalink 参数（与 addTorrent 载荷格式一致）。
func parseAddMetalinkParams(params []any) (payload []byte, options map[string]string, err error) {
	if len(params) == 0 {
		return nil, nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "metalink is required")
	}

	payload, err = decodeTorrentPayload(params[0])
	if err != nil {
		return nil, nil, err
	}

	options = map[string]string{}
	if len(params) >= 2 {
		switch second := params[1].(type) {
		case nil:
		case map[string]any:
			options = parseOptions(second)
		case []any:
			// aria2 三参数形式的 URI 列表对 metalink 无意义，忽略。
			if len(params) >= 3 {
				options = parseOptions(params[2])
			}
		default:
			if params[1] != nil {
				return nil, nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "second param must be options object or null")
			}
		}
	}
	return payload, options, nil
}

func metalinkFilesToAddInputs(files []metalink.File, options map[string]string) ([]task.AddTaskInput, error) {
	out := make([]task.AddTaskInput, 0, len(files))
	for _, file := range files {
		input := task.AddTaskInput{
			URIs:    append([]string(nil), file.URLs...),
			Name:    file.Name,
			Options: cloneOptionMap(options),
			Meta: map[string]string{
				"aria2.metalink": "true",
			},
		}
		if file.Checksum != "" {
			if input.Options == nil {
				input.Options = map[string]string{}
			}
			input.Options["checksum"] = file.Checksum
			input.Meta["aria2.checksum"] = file.Checksum
		}
		if dir := strings.TrimSpace(options["dir"]); dir != "" {
			input.SaveDir = dir
		}
		out = append(out, input)
	}
	return out, nil
}

func (s *Service) addMetalink(ctx context.Context, params []any) (any, error) {
	payload, options, err := parseAddMetalinkParams(params)
	if err != nil {
		return nil, err
	}

	files, err := metalink.Parse(payload)
	if err != nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, err.Error())
	}

	inputs, err := metalinkFilesToAddInputs(files, options)
	if err != nil {
		return nil, err
	}

	gids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		created, err := s.manager.Add(ctx, input)
		if err != nil {
			return nil, err
		}
		gids = append(gids, created.GID)
	}
	return gids, nil
}
