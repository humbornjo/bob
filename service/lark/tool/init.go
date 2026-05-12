package larktool

import (
	_ "embed"

	"github.com/humbornjo/bob/config"
)

var (
	//go:embed tool.cue
	_RAW_TOOL_SCHEMAS string
)

func init() {
	schema, err := config.NewSchema(_RAW_TOOL_SCHEMAS)
	if err != nil {
		panic(err)
	}

	_TOOL_CREATE_MESSAGE_SEND_SCHEMA = config.SchemaMustExtractOpenAPI(schema, ToolCreateMessageSend{})
	_TOOL_CREATE_MESSAGE_REPLY_SCHEMA = config.SchemaMustExtractOpenAPI(schema, ToolCreateMessageReply{})
	_TOOL_LIST_MESSAGES_CHAT_SCHEMA = config.SchemaMustExtractOpenAPI(schema, ToolListMessagesChat{})
	_TOOL_LIST_MESSAGES_THREAD_SCHEMA = config.SchemaMustExtractOpenAPI(schema, ToolListMessagesThread{})
}
