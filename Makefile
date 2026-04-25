.PHONY: all gen clean help
.DEFAULT_GOAL := help

all: gen

gen:
	cue exp gengotypes ./...

run: main.go
	@go run . -c ./local.yaml

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

