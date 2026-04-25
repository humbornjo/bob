package larksvc

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"slices"
	"strings"
	"time"

	"github.com/chyroc/lark"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	anyllm "github.com/mozilla-ai/any-llm-go"

	llmtool "github.com/humbornjo/bob/package/llm/tool"
	larktool "github.com/humbornjo/bob/service/lark/tool"
)

func (s *Service) BuildToolsetPredefined(ctx context.Context) ([]llmtool.Toolx, error) {
	xs := []llmtool.Toolx{
		larktool.NewListMessagesChat(s.larkcli),
		larktool.NewListMessagesThread(s.larkcli),
	}
	for _, mcpcli := range s.mcpclis {
		resp, err := mcpcli.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			return nil, err
		}

		for _, tool := range resp.Tools {
			x := llmtool.FromMcp(mcpcli, tool)
			xs = append(xs, x)
		}
	}
	return xs, nil
}

func (s *Service) BuildMessageSystem() anyllm.Message {
	var buf bytes.Buffer
	err := _SYSTEM_PROMPT_TMPL.Execute(&buf, struct{ Date string }{Date: time.Now().Format("2006-01-02")})
	if err != nil {
		return anyllm.Message{Role: anyllm.RoleSystem, Content: _SYSTEM_PROMPT}
	}
	return anyllm.Message{Role: anyllm.RoleSystem, Content: buf.String()}
}

func (s *Service) BuildMessageUser(
	ctx context.Context,
	content *lark.MessageContent, chatId *string, threadId *string, mentions ...*lark.Mention) (anyllm.Message, error) {
	parts := []anyllm.ContentPart{}

	{
		histchat, err := s.BuildStringChatHistory(ctx, chatId)
		if err != nil {
			return anyllm.Message{}, err
		}
		histthread, err := s.BuildStringChatHistory(ctx, threadId)
		if err != nil {
			return anyllm.Message{}, err
		}
		parts = append(parts, anyllm.ContentPart{Type: "text", Text: histchat + histthread})
	}

	if len(mentions) > 0 {
		str, err := s.BuildStringLarkMentions(ctx, mentions...)
		if err != nil {
			return anyllm.Message{}, err
		}
		parts = append(parts, anyllm.ContentPart{Type: "text", Text: str})
	}

	switch {
	case content.Text != nil:
		parts = append(parts, anyllm.ContentPart{Type: "text", Text: content.Text.Text})
	case content.Image != nil:
		url, err := s.BuildStringImageUrl(ctx, content.Image.ImageKey)
		if err != nil {
			return anyllm.Message{}, err
		}
		parts = append(parts, anyllm.ContentPart{Type: "image_url", ImageURL: &anyllm.ImageURL{URL: url}})
	case content.Post != nil:
		for _, items := range content.Post.Content {
			for _, item := range items {
				switch t := item.(type) {
				case lark.MessageContentPostText:
					parts = append(parts, anyllm.ContentPart{Type: "text", Text: t.Text})
				case lark.MessageContentPostLink:
					parts = append(parts, anyllm.ContentPart{Type: "text", Text: t.Text})
				case lark.MessageContentPostImage:
					url, err := s.BuildStringImageUrl(ctx, t.ImageKey)
					if err != nil {
						return anyllm.Message{}, err
					}
					parts = append(parts, anyllm.ContentPart{Type: "image_url", ImageURL: &anyllm.ImageURL{URL: url}})
				}
			}
		}
	}
	return anyllm.Message{Role: anyllm.RoleUser, Content: parts}, nil
}

func (s *Service) BuildStringLarkUserInfo(ctx context.Context, id string, idType lark.IDType) (string, error) {
	resp, _, err := s.larkcli.Contact.GetUser(ctx, &lark.GetUserReq{
		UserID:     id,
		UserIDType: new(idType),
	})
	if err != nil {
		return "", err
	}
	return resp.User.Name + "@" + resp.User.UserID, nil
}

func (s *Service) BuildStringImageUrl(ctx context.Context, imageKey string) (string, error) {
	resp, _, err := s.larkcli.File.DownloadImage(ctx, &lark.DownloadImageReq{ImageKey: imageKey})
	if err != nil {
		return "", err
	}

	key, err := s.Upload(ctx, resp.File, fmt.Sprintf("%d@%s", time.Now().Nanosecond(), uuid.New()))
	if err != nil {
		return "", err
	}
	return s.PresignUrl(ctx, key)
}

func (s *Service) BuildStringLarkMentions(ctx context.Context, mentions ...*lark.Mention) (string, error) {
	var _tmpl_lark_message_mentions = template.Must(template.New("lark_message_mentions_summary").
		Parse(`<mentions>:{{ range .Mentions }}
- {{ .Name }}@{{ .ID }}
{{ end }}</mentions>
`))

	var b bytes.Buffer
	_ = _tmpl_lark_message_mentions.Execute(&b, struct{ Mentions []*lark.Mention }{Mentions: mentions})
	return b.String(), nil
}

func (s *Service) BuildStringLarkMessage(ctx context.Context, content *lark.MessageContent) (string, error) {
	var builder strings.Builder

	switch {
	case content.Text != nil:
		_, _ = builder.WriteString(content.Text.Text)
	case content.Image != nil:
		url, err := s.BuildStringImageUrl(ctx, content.Image.ImageKey)
		if err != nil {
			return "", err
		}
		_, _ = builder.WriteString("![image](" + url + ")")
	case content.Post != nil:
		for _, items := range content.Post.Content {
			for _, item := range items {
				switch t := item.(type) {
				case lark.MessageContentPostText:
					_, _ = builder.WriteString(t.Text)
				case lark.MessageContentPostLink:
					fmt.Fprintf(&builder, "[%s](%s)", t.Text, t.Href)
				case lark.MessageContentPostImage:
					url, err := s.BuildStringImageUrl(ctx, t.ImageKey)
					if err != nil {
						return "", err
					}
					_, _ = builder.WriteString("![image](" + url + ")")
				}
			}
		}
	}
	return builder.String(), nil
}

func (s *Service) BuildStringChatHistory(ctx context.Context, chatId *string) (string, error) {
	if chatId == nil {
		return "", nil
	}

	historyBuilder := strings.Builder{}
	var _tmpl_list_messages_chat = template.Must(template.New("lark_list_messages_chat_summary").Funcs(template.FuncMap{
		"build_user": func(user *lark.Sender) string {
			res, _ := s.BuildStringLarkUserInfo(ctx, user.ID, user.IDType)
			return res
		},
		"build_content": func(content *lark.MessageContent) string {
			res, _ := s.BuildStringLarkMessage(ctx, content)
			return res
		},
		"build_mentions": func(mentions []*lark.Mention) string {
			res, _ := s.BuildStringLarkMentions(ctx, mentions...)
			return res
		},
	}).Parse(`<chat_history>{{ range .Messages }}
{{ .Mentions | build_mentions }}
{{ .User | build_user }}: {{ .Content | build_content }}
{{ end }}</chat_history>
`))

	resp, _, err := s.larkcli.Message.GetMessageList(ctx, &lark.GetMessageListReq{
		ContainerIDType: lark.ContainerIDTypeChat, ContainerID: *chatId,
		PageSize: new(int64(50)), SortType: new("ByCreateTimeDesc"),
	})
	if err != nil {
		return "", err
	}

	type Message struct {
		Mentions []*lark.Mention
		User     *lark.Sender
		Content  *lark.MessageContent
	}

	slices.Reverse(resp.Items)
	messages := make([]Message, 0, len(resp.Items))
	for _, msg := range resp.Items {
		if msg.Body == nil {
			continue
		}
		content, err := lark.UnwrapMessageContent(msg.MsgType, msg.Body.Content)
		if err != nil {
			continue
		}
		messages = append(messages, Message{Content: content, Mentions: msg.Mentions, User: msg.Sender})
	}
	_ = _tmpl_list_messages_chat.Execute(&historyBuilder, struct{ Messages []Message }{Messages: messages})
	return historyBuilder.String(), nil
}

func (s *Service) BuildStringThreadHistory(ctx context.Context, threadId *string) (string, error) {
	if threadId == nil {
		return "", nil
	}

	historyBuilder := strings.Builder{}
	var _tmpl_list_messages_chat = template.Must(template.New("lark_list_messages_thread_summary").Funcs(template.FuncMap{
		"build_user": func(user *lark.Sender) string {
			res, _ := s.BuildStringLarkUserInfo(ctx, user.ID, user.IDType)
			return res
		},
		"build_content": func(content *lark.MessageContent) string {
			res, _ := s.BuildStringLarkMessage(ctx, content)
			return res
		},
		"build_mentions": func(mentions []*lark.Mention) string {
			res, _ := s.BuildStringLarkMentions(ctx, mentions...)
			return res
		},
	}).Parse(`<thread_history>{{ range .Messages }}
{{ .Mentions | build_mentions }}
{{ .User | build_user }}: {{ .Content | build_content }}
{{ end }}</thread_history>
`))

	resp, _, err := s.larkcli.Message.GetMessageList(ctx, &lark.GetMessageListReq{
		ContainerIDType: lark.ContainerIDTypeThread, ContainerID: *threadId,
		PageSize: new(int64(50)), SortType: new("ByCreateTimeDesc"),
	})
	if err != nil {
		return "", err
	}

	type Message struct {
		Mentions []*lark.Mention
		User     *lark.Sender
		Content  *lark.MessageContent
	}

	slices.Reverse(resp.Items)
	messages := make([]Message, 0, len(resp.Items))
	for _, msg := range resp.Items {
		if msg.Body == nil {
			continue
		}
		content, err := lark.UnwrapMessageContent(msg.MsgType, msg.Body.Content)
		if err != nil {
			continue
		}
		messages = append(messages, Message{Content: content, Mentions: msg.Mentions, User: msg.Sender})
	}
	_ = _tmpl_list_messages_chat.Execute(&historyBuilder, struct{ Messages []Message }{Messages: messages})
	return historyBuilder.String(), nil
}
