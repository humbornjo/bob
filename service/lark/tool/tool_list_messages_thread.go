package larktool

import (
	"bytes"
	"context"
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
var _ llmtool.Params = (*ToolListMessagesThread)(nil)

var _TOOL_LIST_MESSAGES_THREAD_SCHEMA map[string]any

func (t *ToolListMessagesThread) String() string {
	return "ToolListMessagesThread"
}

func (t *ToolListMessagesThread) Schema() map[string]any {
	return _TOOL_LIST_MESSAGES_THREAD_SCHEMA
}

// Toolx Implementation ----------------------------------------------
var _ llmtool.Toolx = (*toolListMessagesThread)(nil)

func NewListMessagesThread(larkcli *lark.Lark) llmtool.Toolx {
	return &toolListMessagesThread{larkcli: larkcli}
}

type toolListMessagesThread struct {
	ToolListMessagesThread `json:",inline"`

	larkcli *lark.Lark `json:"-"`
}

func (t *toolListMessagesThread) Tool() anyllm.Tool {
	return anyllm.Tool{
		Type:     "function",
		Function: t.Function(),
	}
}

func (t *toolListMessagesThread) Function() anyllm.Function {
	return anyllm.Function{
		Name: "lark_list_messages_thread",
		Description: "Call this tool to retrieve the message history of a specific thread. " +
			"Use this tool when: " +
			"1. The user refers to a threaded conversation that needs context, " +
			"2. You need to understand the full thread to respond accurately, " +
			"3. The discussion is happening in a thread and you need to see all replies.",
		Parameters: _TOOL_LIST_MESSAGES_THREAD_SCHEMA,
	}
}

func (t *toolListMessagesThread) Execute(ctx context.Context, args string, opts ...llmtool.Option) (string, error) {
	toolcfg := llmtool.NewConfig(opts...)

	// Get context metadata
	var threadID string
	var pageToken *string
	if val, ok := toolcfg.Metadata["thread_id"].(string); ok {
		threadID = val
	}
	if threadID == "" {
		return "", errors.New("thread_id is required for listing lark thread history")
	}
	if val, ok := toolcfg.Metadata["lark_list_history_thread_page_token"].(string); ok && val != "" {
		pageToken = new(val)
	}

	resp, _, err := t.larkcli.Message.GetMessageList(ctx, &lark.GetMessageListReq{
		ContainerIDType: lark.ContainerIDTypeThread, ContainerID: threadID,
		PageSize: new(int64(10)), PageToken: pageToken, SortType: new("ByCreateTimeDesc"),
	})
	if err != nil {
		return "", err
	}
	toolcfg.Metadata["lark_list_history_thread_page_token"] = resp.PageToken

	tmpl := template.Must(template.New("lark_list_messages_thread_summary").Funcs(template.FuncMap{
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

func (t *toolListMessagesThread) ExecuteStream(ctx context.Context, args string, opts ...llmtool.Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield(t.Execute(ctx, args, opts...))
	}
}
