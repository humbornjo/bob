package larksvc

import (
	llmmcp "github.com/humbornjo/bob/package/llm/mcp:llmmcp"
	"github.com/humbornjo/bob/package/llm"
)

#Config: {
	storage!: {
		folder!: (string & !="") | *"/"
	}

	model: {
		provider!: llm.#Provider
		name!:     string
		mcpservers: [...llmmcp.#ConfigMCP] @go("McpServers")
	}
}
