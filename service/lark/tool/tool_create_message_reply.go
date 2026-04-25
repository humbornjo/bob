package larktool

import (
	"context"
	"iter"

	anyllm "github.com/mozilla-ai/any-llm-go"

	llmtool "github.com/humbornjo/bob/package/llm/tool"
)

// Params Implementation ---------------------------------------------
var _ llmtool.Params = (*ToolCreateMessageReply)(nil)

var _TOOL_CREATE_MESSAGE_REPLY_SCHEMA map[string]any

func (t *ToolCreateMessageReply) String() string {
	return "ToolCreateMessageReply"
}

func (t *ToolCreateMessageReply) Schema() map[string]any {
	return _TOOL_CREATE_MESSAGE_REPLY_SCHEMA
}

// Toolx Implementation ----------------------------------------------
var _ llmtool.Toolx = (*ToolCreateMessageReply)(nil)

func NewCreateMessageReply() llmtool.Toolx {
	return &ToolCreateMessageReply{}
}

func (t *ToolCreateMessageReply) Name() string {
	return "lark_create_message_reply"
}

func (t *ToolCreateMessageReply) Tool() anyllm.Tool {
	return anyllm.Tool{
		Type:     "function",
		Function: t.Function(),
	}
}

func (t *ToolCreateMessageReply) Function() anyllm.Function {
	return anyllm.Function{
		Name: t.Name(),
		Description: "Call this tool to reply to a single user's message. " +
			"This quotes the user's message being replied to. " +
			"Use this reply method when: " +
			"1. The response is a direct answer to the user's specific question, " +
			"2. Continuing an ongoing conversation, " +
			"3. Providing feedback or clarification on the user's message, " +
			"4. The message is intended for the current user only and not for broadcast.",
		Parameters: _TOOL_CREATE_MESSAGE_REPLY_SCHEMA,
	}
}

func (t *ToolCreateMessageReply) Execute(ctx context.Context, args string, opts ...llmtool.Option) (string, error) {
	return "", nil
}

func (t *ToolCreateMessageReply) ExecuteStream(ctx context.Context, args string, opts ...llmtool.Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {}
}
