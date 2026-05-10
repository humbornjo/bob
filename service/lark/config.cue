package larksvc

import (
	llmmcp "github.com/humbornjo/bob/package/llm/mcp:llmmcp"
	"github.com/humbornjo/bob/package/llm"
)

#Config: {
	// Storage related config for lark service
	storage!: {
		// Folder name as the sub directory of the root storage directory
		folder!: (string & !="") | *"/"
	}

	// Large Language Model related config
	model: {
		// Provider standard to be used
		provider!: llm.#Provider
		// Model name
		name!: string
		// MCP servers config
		mcpservers: [...llmmcp.#ConfigMCP] @go("McpServers")
	}
}
