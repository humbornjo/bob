package larktool

import (
	_ "embed"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/openapi"
)

var (
	//go:embed tool.cue
	_TOOL_SCHEMAS_RAW []byte
	_TOOL_SCHEMAS     cue.Value
)

func init() {
	cuetex := cuecontext.New()
	_TOOL_SCHEMAS = cuetex.CompileBytes(_TOOL_SCHEMAS_RAW)
	f, err := openapi.Generate(_TOOL_SCHEMAS, &openapi.Config{})
	if err != nil {
		panic(err)
	}
	topValue := cuetex.BuildFile(f)
	if err := topValue.Err(); err != nil {
		panic(err)
	}

	var schemas map[string]map[string]any
	{
		var document struct {
			Components struct {
				Schemas map[string]map[string]any `json:"schemas"`
			} `json:"components"`
		}

		if err := topValue.Decode(&document); err != nil {
			panic(err)
		}

		schemas = document.Components.Schemas
	}

	ensure_schema := func(key string) map[string]any {
		params, ok := schemas[key]
		if !ok {
			panic("missing schema: " + key)
		}
		if _, ok := params["properties"]; !ok {
			params["properties"] = map[string]any{}
		}

		return params
	}
	_TOOL_CREATE_MESSAGE_SEND_SCHEMA = ensure_schema((&ToolCreateMessageSend{}).String())
	_TOOL_CREATE_MESSAGE_REPLY_SCHEMA = ensure_schema((&ToolCreateMessageReply{}).String())
	_TOOL_LIST_MESSAGES_CHAT_SCHEMA = ensure_schema((&ToolListMessagesChat{}).String())
	_TOOL_LIST_MESSAGES_THREAD_SCHEMA = ensure_schema((&ToolListMessagesThread{}).String())
}
