package larksvc

import (
	_ "embed"
	"html/template"
	"io"
	"net/http"

	"github.com/chyroc/lark"
	mcpclient "github.com/mark3labs/mcp-go/client"
	anyllm "github.com/mozilla-ai/any-llm-go"

	"github.com/humbornjo/bob/package/llm"
	"github.com/humbornjo/bob/package/storage"
)

var (
	//go:embed static/system_prompt.md
	_SYSTEM_PROMPT      string
	_SYSTEM_PROMPT_TMPL = template.Must(template.New("system_prompt").Parse(_SYSTEM_PROMPT))
)

type Service struct {
	storage.Storage

	model   string
	larkcli *lark.Lark
	mcpclis []*mcpclient.Client
}

func (s *Service) HandleSendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	buf, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	_ = r.Body.Close()

	messages := []anyllm.Message{
		{
			Role:    anyllm.RoleUser,
			Content: string(buf),
		},
	}

	params := &anyllm.CompletionParams{
		Model:    "kimi-k2.6",
		Messages: messages,
		Stream:   true,
	}
	for _, it := range llm.AgentStream(ctx, params) {
		for chunk, err := range it {
			if err != nil {
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				delta := chunk.Choices[0].Delta.Content
				_, _ = w.Write([]byte("\n[DELTA]: "))
				_, _ = w.Write([]byte(delta))
			}
			if reason := chunk.Choices[0].FinishReason; reason != "" {
				_, _ = w.Write([]byte("\n[FINISH_REASON]: "))
				_, _ = w.Write([]byte(reason))
				return
			}
		}
	}
}
