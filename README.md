# Bob

Bob is a experimental agent framework to explore the extreme of handling large language model development using Go lang.
The idea seeds from [cue-lang](https://cuelang.org/) and its experimental command
[`gengotypes`](https://cuelang.org/docs/howto/generate-go-types-from-cue-definitions/) when I realize how easy and fun it
could be to describe the tools and config in a strict config-oriented way with Go.

# Services

## Lark Service

Lark service provide a connection on Lark bot which is able to

- **Chat**: `p2p`/`group`/`thread` chat with a well managed context mechanism.

### Prerequisite

| Variable          | Description                        | Default Value            |
| ----------------- | ---------------------------------- | ------------------------ |
| `LARK_DOMAIN`     | Lark Domain                        | "https://open.feishu.cn" |
| `LARK_APP_ID`     | Lark App ID from open platform     | -                        |
| `LARK_APP_SECRET` | Lark App Secret from open platform | -                        |

- Create a lark bot in the [openplatform](https://open.feishu.cn/app?lang=en-US) (_App ID_ and _App Secret_).

# Todo

[ ] aggregate multiple OpenAPI documents to display the scattered tool params definitions.
[ ] integrate some sandbox standard and fine-grain session management accordingly.

# Disclaimer

- package `llmskill` steals code from [hermes-agent](https://github.com/nousresearch/hermes-agent)
