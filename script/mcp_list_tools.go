package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	// Get app credentials from environment variables / 从环境变量获取应用凭证
	appId := os.Getenv("LARK_APP_ID")
	appSecret := os.Getenv("LARK_APP_SECRET")
	larkDomain := os.Getenv("LARK_DOMAIN")

	if appId == "" || appSecret == "" {
		log.Println("警告: APP_ID 或 APP_SECRET 未设置，将使用默认的MCP服务器配置")
		log.Println("Warning: APP_ID or APP_SECRET not set, using default MCP server configuration")
	}

	mcpcli, err := client.NewStdioMCPClient(
		"npx",
		[]string{},
		"-y", "@larksuiteoapi/lark-mcp", "mcp", "-a", appId, "-s", appSecret, "-d", larkDomain, "--token-mode", "tenant_access_token",
		// 你可以自定义开启的 Tools 或者 Presets / You can custom enable tools or presets here
		// '-t',
		// 'bitable.v1.app.create,bitable.v1.appTable.create',
	)

	if err != nil {
		log.Fatal(err)
	}

	// Start and initialize the MCP client / 启动并初始化 MCP 客户端
	ctx := context.Background()
	err = mcpcli.Start(ctx)
	if err != nil {
		log.Fatal("Failed to start MCP client", err)
	}
	_, err = mcpcli.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		log.Fatal("Failed to initialize MCP client", err)
	}

	resp, err := mcpcli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatal("Failed to list tools", err)
	}

	jsonb, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(jsonb))

	r1 := mcp.CallToolRequest{
		Request: mcp.Request{
			Params: mcp.RequestParams{
				Meta: &mcp.Meta{
					AdditionalFields: map[string]any{
						"foo": "bar",
					},
				},
			},
		},
	}

	jsonbr1, err := json.MarshalIndent(r1, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(jsonbr1))

	r2 := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Meta: &mcp.Meta{
				AdditionalFields: map[string]any{
					"foo": "bar",
				},
			},
		},
	}

	jsonbr2, err := json.MarshalIndent(r2, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(jsonbr2))
}
