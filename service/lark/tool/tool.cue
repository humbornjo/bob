package larktool

#ToolCreateMessageReply: {}

#ToolCreateMessageSend: {}

#ToolListMessagesChat: {
	// start time to search history, specify it only when needed
	start_time?: string @go(StartTime,optional=nillable)
	// end time to search history, specify it only when needed
	end_time?: string @go(EndTime,optional=nillable)
}

#ToolListMessagesThread: {}
