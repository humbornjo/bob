package llmtool

import (
	"context"
	"encoding/json"
	"iter"
	"strings"

	llmmcp "github.com/humbornjo/bob/package/llm/mcp"
	"github.com/mark3labs/mcp-go/mcp"
	anyllm "github.com/mozilla-ai/any-llm-go"
)

func FromMCP(mcpcli *llmmcp.Client, tool mcp.Tool) Toolx {
	params := map[string]any{
		"type":       tool.InputSchema.Type,
		"properties": tool.InputSchema.Properties,
	}

	if tool.InputSchema.Defs != nil {
		params["$defs"] = tool.InputSchema.Defs
	}
	if tool.InputSchema.Required != nil {
		params["required"] = tool.InputSchema.Required
	}
	if tool.InputSchema.AdditionalProperties != nil {
		params["additionalProperties"] = tool.InputSchema.AdditionalProperties
	}

	return &toolmcp{
		mcpcli: mcpcli,
		name:   tool.Name, function: anyllm.Function{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		},
	}
}

type toolmcp struct {
	mcpcli *llmmcp.Client

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

	var parsedArgs = make(map[string]any)
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", err
	}

	resp, err := t.mcpcli.CallTool(ctx, mcp.CallToolRequest{
		Header: nil,
		Params: mcp.CallToolParams{
			Name:      t.name,
			Arguments: parsedArgs,
			Meta:      &mcp.Meta{AdditionalFields: toolcfg.Metadata},
		},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Content) == 0 {
		return "tool returned no content", nil
	}

	builder := strings.Builder{}
	for _, content := range resp.Content {
		switch cont := content.(type) {
		case mcp.TextContent:
			builder.WriteString(cont.Text)
		case mcp.ImageContent:
		case mcp.AudioContent:
		case mcp.EmbeddedResource:
		}
	}
	return builder.String(), nil
}

func (t *toolmcp) ExecuteStream(ctx context.Context, args string, opts ...Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield(t.Execute(ctx, args, opts...))
	}
}
