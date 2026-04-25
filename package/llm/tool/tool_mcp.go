package llmtool

import (
	"context"
	"iter"
	"strings"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	anyllm "github.com/mozilla-ai/any-llm-go"
)

func FromMcp(mcpcli *mcpclient.Client, tool mcp.Tool) Toolx {
	return &toolmcp{
		mcpcli: mcpcli,

		name: tool.Name, function: anyllm.Function{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema.Defs,
		},
	}
}

type toolmcp struct {
	mcpcli *mcpclient.Client

	name     string
	function anyllm.Function
}

func (t *toolmcp) Name() string {
	return t.name
}

func (t *toolmcp) Function() anyllm.Function {
	return t.function
}

func (t *toolmcp) Tool() anyllm.Tool {
	return anyllm.Tool{
		Type:     "function",
		Function: t.Function(),
	}
}

func (t *toolmcp) Execute(ctx context.Context, args string, opts ...Option) (string, error) {
	toolcfg := NewConfig(opts...)
	resp, err := t.mcpcli.CallTool(ctx, mcp.CallToolRequest{
		Header: nil,
		Params: mcp.CallToolParams{
			Name:      t.name,
			Arguments: args,
			Meta:      &mcp.Meta{AdditionalFields: toolcfg.Metadata},
		},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "tool returned no content", nil
	}

	sb := strings.Builder{}
	for _, content := range resp.Content {
		switch cont := content.(type) {
		case mcp.TextContent:
			sb.WriteString(cont.Text)
		}
	}
	return sb.String(), nil
}

func (t *toolmcp) ExecuteStream(ctx context.Context, args string, opts ...Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield(t.Execute(ctx, args, opts...))
	}
}
