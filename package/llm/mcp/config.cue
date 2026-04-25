package llmmcp

#ToolExtension: {
	// The name of the tool
	name!: string @go("Name")
	// Go template for the description, the original description will
	// automatically be rendered into {{ .Description }}.
	description_template?: string @go("DescriptionTemplate")
}

#TransportType: string & (*"streamable_http" | "sse")

#TransportSSE: {
	// The endpoint of the mcp server
	endpoint!: string
	// The headers of the request
	headers?: {[string]: string}
}

#TransportStreamableHTTP: {
	// The endpoint of the mcp server
	endpoint!: string
	// The headers of the request
	headers?: {[string]: string}
}

#ConfigTransport: {
	// The type of the transport
	type!: #TransportType
	// The config of the transport
	config!: #TransportSSE | #TransportStreamableHTTP @go("Config",type=any)
}

#ConfigMCP: {
	// The transport type of the mcp server
	transport!: #ConfigTransport
	// A list of allowed tools, if empty, all tools except the forbidden
	// are allowed. If enabled_tools and ddisabled_tools overlapped, the
	// overlapped tools are disabled.
	enabled_tools: [...string] @go("EnabledTools")
	// A list of forbidden tools, if empty, refer to the logic of enabled.
	disabled_tools: [...string] @go("DisabledTools")
	// A list of tool extensions
	tool_extensions: [...#ToolExtension] @go("ToolExtensions")
}
