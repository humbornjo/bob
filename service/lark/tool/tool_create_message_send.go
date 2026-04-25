package larktool

import (
	"context"
	"iter"

	anyllm "github.com/mozilla-ai/any-llm-go"

	llmtool "github.com/humbornjo/bob/package/llm/tool"
)

// Params Implementation ---------------------------------------------
var _ llmtool.Params = (*ToolCreateMessageSend)(nil)

var _TOOL_CREATE_MESSAGE_SEND_SCHEMA map[string]any

func (t *ToolCreateMessageSend) String() string {
	return "ToolCreateMessageSend"
}

func (t *ToolCreateMessageSend) Schema() map[string]any {
	return _TOOL_CREATE_MESSAGE_SEND_SCHEMA
}

// Toolx Implementation ----------------------------------------------
var _ llmtool.Toolx = (*ToolCreateMessageSend)(nil)

func NewCreateMessageSend() llmtool.Toolx {
	return &ToolCreateMessageSend{}
}

func (t *ToolCreateMessageSend) Name() string {
	return "lark_send_message"
}

func (t *ToolCreateMessageSend) Tool() anyllm.Tool {
	return anyllm.Tool{
		Type:     "function",
		Function: t.Function(),
	}
}

func (t *ToolCreateMessageSend) Function() anyllm.Function {
	return anyllm.Function{
		Name: t.Name(),
		Description: "Call this tool to send a message to the current Lark conversation. " +
			"Unlike reply, this sends a standalone message without quoting any previous message. " +
			"Use this send method when: " +
			"1. Starting a new conversation or topic, " +
			"2. Broadcasting information to all participants in the chat, " +
			"3. The message is not a direct response to any specific user's question, " +
			"4. Sending announcements or general updates that don't require context from previous messages.",
		Parameters: _TOOL_CREATE_MESSAGE_SEND_SCHEMA,
	}
}

func (t *ToolCreateMessageSend) Execute(ctx context.Context, args string, opts ...llmtool.Option) (string, error) {
	return "", nil
}

func (t *ToolCreateMessageSend) ExecuteStream(ctx context.Context, args string, opts ...llmtool.Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {}
}
