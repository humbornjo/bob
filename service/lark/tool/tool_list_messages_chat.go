package larktool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"iter"
	"strconv"
	"time"

	"github.com/chyroc/lark"
	anyllm "github.com/mozilla-ai/any-llm-go"

	llmtool "github.com/humbornjo/bob/package/llm/tool"
)

// Params Implementation ---------------------------------------------
var _ llmtool.Params = (*ToolListMessagesChat)(nil)

var _TOOL_LIST_MESSAGES_CHAT_SCHEMA map[string]any

func (t *ToolListMessagesChat) String() string {
	return "ToolListMessagesChat"
}

func (t *ToolListMessagesChat) Schema() map[string]any {
	return _TOOL_LIST_MESSAGES_CHAT_SCHEMA
}

// Toolx Implementation ----------------------------------------------
var _ llmtool.Toolx = (*toolListMessagesChat)(nil)

func NewListMessagesChat(larkcli *lark.Lark) llmtool.Toolx {
	return &toolListMessagesChat{larkcli: larkcli}
}

type toolListMessagesChat struct {
	larkcli *lark.Lark
}

func (t *toolListMessagesChat) Name() string {
	return "lark_list_messages_chat"
}

func (t *toolListMessagesChat) Tool() anyllm.Tool {
	return anyllm.Tool{
		Type:     "function",
		Function: t.Function(),
	}
}

func (t *toolListMessagesChat) Function() anyllm.Function {
	return anyllm.Function{
		Name: t.Name(),
		Description: "Call this tool to retrieve the message history of the current chat. " +
			"Use this tool when: " +
			"1. You need context from previous messages to answer a question, " +
			"2. The user refers to something discussed earlier in the conversation, " +
			"3. You need to understand the full conversation thread to provide accurate assistance.",
		Parameters: _TOOL_LIST_MESSAGES_CHAT_SCHEMA,
	}
}

func (t *toolListMessagesChat) Execute(ctx context.Context, args string, opts ...llmtool.Option) (string, error) {
	toolcfg := llmtool.NewConfig(opts...)

	// Get context metadata
	var chatId string
	var pageToken *string
	if val, ok := toolcfg.Metadata["chat_id"].(string); ok {
		chatId = val
	}
	if chatId == "" {
		return "", errors.New("chat_id is required for listing lark chat history")
	}
	if val, ok := toolcfg.Metadata["lark_list_history_chat_page_token"].(string); ok && val != "" {
		pageToken = new(val)
	}

	params := ToolListMessagesChat{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", err
	}

	var startTime, endTime *string
	if params.StartTime != nil && *params.StartTime != "" {
		ts, err := time.Parse(time.RFC3339, *params.StartTime)
		if err != nil {
			return "", err
		}
		startTime = new(strconv.FormatInt(ts.Unix(), 10))
	}
	if params.EndTime != nil && *params.EndTime != "" {
		ts, err := time.Parse(time.RFC3339, *params.EndTime)
		if err != nil {
			return "", err
		}
		endTime = new(strconv.FormatInt(ts.Unix(), 10))
	}
	resp, _, err := t.larkcli.Message.GetMessageList(ctx, &lark.GetMessageListReq{
		ContainerIDType: lark.ContainerIDTypeChat, ContainerID: chatId,
		StartTime: startTime, EndTime: endTime,
		PageSize: new(int64(10)), PageToken: pageToken, SortType: new("ByCreateTimeDesc"),
	})
	if err != nil {
		return "", err
	}
	toolcfg.Metadata["lark_list_history_chat_page_token"] = resp.PageToken

	tmpl := template.Must(template.New("lark_list_messages_chat_summary").Funcs(template.FuncMap{
		"formatTime": func(ts string) string {
			if i, err := strconv.ParseInt(ts, 10, 64); err == nil {
				return time.UnixMilli(i).Format(time.RFC3339)
			}
			return ts
		},
	}).Parse(`{{range .Messages}}---
Time: {{.CreateTime | formatTime}}
User: {{.Sender.ID}}
Content: {{.Body.Content}}
{{if .Mentions}}Mentions:{{range .Mentions}} @{{.ID}}{{end}}
{{end}}
{{end}}{{if .HasMore}}---
[Has more messages, use the tool again to see older messages]{{end}}`))
	buffer := bytes.Buffer{}
	if err := tmpl.Execute(&buffer, struct {
		HasMore  bool
		Messages []*lark.GetMessageListRespItem
	}{HasMore: resp.HasMore, Messages: resp.Items}); err != nil {
		return "", err
	}

	return buffer.String(), nil
}

func (t *toolListMessagesChat) ExecuteStream(ctx context.Context, args string, opts ...llmtool.Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield(t.Execute(ctx, args, opts...))
	}
}
