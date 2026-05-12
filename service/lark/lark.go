package larksvc

import (
	_ "embed"
	"html/template"

	"github.com/chyroc/lark"
	anyllm "github.com/mozilla-ai/any-llm-go"

	"github.com/humbornjo/bob/package/llm"
	llmmcp "github.com/humbornjo/bob/package/llm/mcp"
	"github.com/humbornjo/bob/package/storage"
	"github.com/humbornjo/mizu/mizuoai"
)

var (
	//go:embed static/system_prompt.md
	_SYSTEM_PROMPT      string
	_SYSTEM_PROMPT_TMPL = template.Must(template.New("system_prompt").Parse(_SYSTEM_PROMPT))
)

type Service struct {
	storage.Storage

	model    string
	provider anyllm.Provider
	larkcli  *lark.Lark
	mcpclis  []*llmmcp.Client

	appname, appid, openid string
}

type HandleSendMessageInput struct {
	Content string `mizu:"body"`
}

type HandleSendMessageOutput = string

func (s *Service) HandleSendMessage(tx mizuoai.Tx[HandleSendMessageOutput], rx mizuoai.Rx[HandleSendMessageInput]) {
	ctx := rx.Context()

	input, err := rx.MizuRead()
	if err != nil {
		_ = tx.MizuWrite(new(err.Error()))
		return
	}

	messages := []anyllm.Message{{Role: anyllm.RoleUser, Content: input.Content}}
	params := &anyllm.CompletionParams{
		Model:           s.model,
		Stream:          true,
		Messages:        messages,
		ReasoningEffort: anyllm.ReasoningEffortNone,
	}

	for _, it := range llm.AgentStream(ctx, s.provider, params) {
		for chunk, err := range it {
			if err != nil {
				_ = tx.MizuWrite(new(err.Error()))
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]
			if content := choice.Delta.Content; content != "" {
				_ = tx.MizuWrite(new("\n[DELTA_CONTENT]: " + content))
			}
			if reasoning := choice.Delta.Reasoning; reasoning != nil {
				_ = tx.MizuWrite(new("\n[DELTA_REASONING]: " + reasoning.Content))
			}

			if reason := chunk.Choices[0].FinishReason; reason != "" {
				_ = tx.MizuWrite(new("\n[FINISH_REASON]: " + reason))
				return
			}
		}
	}
}
