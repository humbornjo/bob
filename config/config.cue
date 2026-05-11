package config

#Config: {
	env!:   string & (*"dev" | "test" | "staging" | "prod" | "local")
	level!: string & (*"DEBUG" | "INFO" | "WARN" | "ERROR" | "FATAL")
	port!:  (string & =~"^[1-9][0-9]{0,4}$" | *"80")

	volcengine!: {
		tos!: {
			endpoint!:   string & =~"^https?://"
			region!:     string
			bucket!:     string
			access_key!: string @go("AccessKey")
			secret_key!: string @go("SecretKey")
		}
	}
}
