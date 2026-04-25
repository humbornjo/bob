package llmmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"slices"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	TRANSPORT_SSE             TransportType = "sse"
	TRANSPORT_STREAMABLE_HTTP TransportType = "streamable_http"
)

type Client struct {
	*mcpclient.Client
	enabled    []string
	disabled   []string
	extensions map[string]struct {
		Template *template.Template
	}
}

type Option func(*Client)

type Transport interface {
	TransportStreamableHTTP | TransportSSE
}

func WithEnabledTools(enabled ...string) Option {
	return func(cli *Client) {
		cli.enabled = enabled
	}
}

func WithDisabledTools(disabled ...string) Option {
	return func(cli *Client) {
		cli.disabled = disabled
	}
}

func WithToolExtensions(extensions ...ToolExtension) Option {
	return func(cli *Client) {
		for _, ext := range extensions {
			tmpl := template.Must(template.New(ext.Name).Parse(ext.DescriptionTemplate))
			cli.extensions[ext.Name] = struct{ Template *template.Template }{Template: tmpl}
		}
	}
}

func NewClient(config ConfigTransport, opts ...Option) (cli *Client, err error) {
	parse := func(data any, val any) error {
		jsonb, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return json.Unmarshal(jsonb, val)
	}

	var inner *mcpclient.Client
	switch config.Type {
	case TRANSPORT_SSE:
		var t TransportSSE
		if err := parse(config.Config, &t); err != nil {
			return nil, err
		}
		opts := make([]mcptransport.ClientOption, 0, 4)
		if t.Headers != nil {
			opts = append(opts, mcptransport.WithHeaders(t.Headers))
		}
		inner, err = mcpclient.NewSSEMCPClient(t.Endpoint, opts...)
	case TRANSPORT_STREAMABLE_HTTP:
		var t TransportStreamableHTTP
		if err := parse(config.Config, &t); err != nil {
			return nil, err
		}
		opts := make([]mcptransport.StreamableHTTPCOption, 0, 4)
		if t.Headers != nil {
			opts = append(opts, mcptransport.WithHTTPHeaders(t.Headers))
		}
		inner, err = mcpclient.NewStreamableHttpClient(t.Endpoint, opts...)
	default:
		return nil, errors.New("unknown transport type")
	}
	if err != nil {
		return nil, err
	}

	cli = &Client{Client: inner, extensions: make(map[string]struct{ Template *template.Template })}
	for _, opt := range opts {
		opt(cli)
	}
	return cli, nil
}

func (cli *Client) ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	resp, err := cli.Client.ListTools(ctx, request)
	if err != nil {
		return nil, err
	}

	tools := make([]mcp.Tool, 0, len(resp.Tools))
	for _, tool := range resp.Tools {
		if len(cli.enabled) > 0 && !slices.Contains(cli.enabled, tool.Name) {
			continue
		}
		if len(cli.disabled) > 0 && slices.Contains(cli.disabled, tool.Name) {
			continue
		}

		ext, ok := cli.extensions[tool.Name]
		if !ok {
			tools = append(tools, tool)
			continue
		}

		if ext.Template != nil {
			buffer := bytes.Buffer{}
			desc := tool.Description
			err := ext.Template.Execute(&buffer, struct{ Description string }{
				Description: desc,
			})
			if err == nil {
				return nil, err
			}
			tool.Description = buffer.String()
		}
		tools = append(tools, tool)
	}

	resp.Tools = tools
	return resp, nil
}
