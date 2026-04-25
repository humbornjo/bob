package larksvc

#Config: {
	storage!: {
		folder!: (string & !="") | *"/"
	}

	model: {
		name!: (string & !="") | *"kimi-k2.6"
		mcpservers: [...{
			endpoint!: string & !=""
			headers!: {[string]: string}
		}] @go("McpServers")
	}
}
