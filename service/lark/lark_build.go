package larksvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chyroc/lark"
	"github.com/google/uuid"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/mark3labs/mcp-go/mcp"
	anyllm "github.com/mozilla-ai/any-llm-go"

	"github.com/humbornjo/bob/package/llm"
	llmskill "github.com/humbornjo/bob/package/llm/skill"
	llmtool "github.com/humbornjo/bob/package/llm/tool"
	larksock "github.com/humbornjo/bob/service/lark/sock"
	larktool "github.com/humbornjo/bob/service/lark/tool"
)

func (s *Service) BuildCompletionParams(ctx context.Context, messages []anyllm.Message, event *larkim.P2MessageReceiveV1,
) (larksock.Socket, anyllm.CompletionParams, error) {
	toolCreateMessageSend := larktool.NewCreateMessageSend()
	toolCreateMessageReply := larktool.NewCreateMessageReply()
	params := anyllm.CompletionParams{
		Model:           s.model,
		Messages:        messages,
		ToolChoice:      "required",
		ReasoningEffort: anyllm.ReasoningEffortNone,
		Tools:           []anyllm.Tool{toolCreateMessageSend.Tool(), toolCreateMessageReply.Tool()},
	}

	var socket larksock.Socket
	completion, err := llm.Completion(ctx, s.provider, params)
	if err != nil {
		slog.ErrorContext(ctx, "failed to call chat completion", "err", err)
		return nil, anyllm.CompletionParams{}, err
	}
	switch completion.Choices[0].Message.ToolCalls[0].Function.Name {
	case toolCreateMessageSend.Function().Name:
		socket = larksock.NewSendSocket(s.larkcli, *event.Event.Message.ChatId)
	case toolCreateMessageReply.Function().Name:
		socket = larksock.NewReplySocket(s.larkcli, *event.Event.Message.MessageId)
	}
	defer socket.Close(ctx, nil) // nolint: errcheck

	params.Stream = true
	params.ToolChoice = "auto"
	params.ParallelToolCalls = new(true)
	params.ReasoningEffort = anyllm.ReasoningEffortMedium

	return socket, params, nil
}

func (s *Service) BuildToolsetPredefined(ctx context.Context) ([]llmtool.Toolx, error) {
	xs := []llmtool.Toolx{
		larktool.NewListMessagesChat(s.larkcli),
		larktool.NewListMessagesThread(s.larkcli),
		llmskill.NewToolSkillView(),
		llmskill.NewToolSkillsList(),
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

	slog.DebugContext(ctx, "built toolset", "count", len(xs))
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
	content *lark.MessageContent, chatID *string, threadId *string, mentions ...*lark.Mention) (anyllm.Message, error) {
	contentparts := []anyllm.ContentPart{}

	{
		histchat, err := s.BuildStringLarkHistory(ctx, chatID, lark.ContainerIDTypeChat)
		if err != nil {
			return anyllm.Message{}, err
		}
		slog.DebugContext(ctx, "chat history", "chat_id", chatID, "history", histchat)
		histthread, err := s.BuildStringLarkHistory(ctx, threadId, lark.ContainerIDTypeThread)
		if err != nil {
			return anyllm.Message{}, err
		}
		slog.DebugContext(ctx, "thread history", "thread_id", threadId, "history", histthread)
		contentparts = append(contentparts, anyllm.ContentPart{Type: "text", Text: histchat + histthread})
	}

	if len(mentions) > 0 {
		str, err := s.BuildStringLarkMentions(ctx, mentions...)
		if err != nil {
			return anyllm.Message{}, err
		}
		contentparts = append(contentparts, anyllm.ContentPart{Type: "text", Text: str})
	}

	if parts, err := s.BuildContentPartsLarkMessage(ctx, content); err != nil {
		return anyllm.Message{}, err
	} else {
		contentparts = append(contentparts, parts...)
	}

	return anyllm.Message{Role: anyllm.RoleUser, Content: contentparts}, nil
}

func (s *Service) BuildContentPartsLarkMessage(ctx context.Context, content *lark.MessageContent,
) ([]anyllm.ContentPart, error) {
	var parts []anyllm.ContentPart
	switch content.MsgType {
	case lark.MsgTypeText:
		parts = append(parts, anyllm.ContentPart{Type: "text", Text: content.Text.Text})
	case lark.MsgTypeImage:
		url, err := s.BuildStringLarkImageURL(ctx, content.Image.ImageKey)
		if err != nil {
			return nil, err
		}
		parts = append(parts, anyllm.ContentPart{Type: "image_url", ImageURL: &anyllm.ImageURL{URL: url}})
	case lark.MsgTypePost:
		for _, items := range content.Post.Content {
			for _, item := range items {
				switch t := item.(type) {
				case lark.MessageContentPostText:
					parts = append(parts, anyllm.ContentPart{Type: "text", Text: t.Text})
				case lark.MessageContentPostLink:
					parts = append(parts, anyllm.ContentPart{Type: "text", Text: t.Text})
				case lark.MessageContentPostImage:
					url, err := s.BuildStringLarkImageURL(ctx, t.ImageKey)
					if err != nil {
						return nil, err
					}
					parts = append(parts, anyllm.ContentPart{Type: "image_url", ImageURL: &anyllm.ImageURL{URL: url}})
				}
			}
		}
	}
	return parts, nil
}

func (s *Service) BuildStringLarkUser(ctx context.Context, id string, idType lark.IDType) (string, error) {
	resp, _, err := s.larkcli.Contact.GetUser(ctx, &lark.GetUserReq{
		UserID:     id,
		UserIDType: new(idType),
	})
	if err != nil {
		slog.DebugContext(ctx, "failed to get user info", "err", err, "id", id, "id_type", idType)
		return "", err
	}
	return resp.User.Name + "@" + resp.User.UserID, nil
}

func (s *Service) BuildStringLarkApp(ctx context.Context, id string, idType lark.IDType) (string, error) {
	resp, _, err := s.larkcli.Application.GetApplication(ctx, &lark.GetApplicationReq{
		AppID: id,
		Lang:  "en_us",
	})
	if err != nil {
		slog.DebugContext(ctx, "failed to get app info", "err", err, "id", id, "id_type", idType)
		return "", err
	}
	return resp.App.AppName + "@" + resp.App.AppID, nil
}

func (s *Service) BuildStringLarkImageURL(ctx context.Context, imageKey string) (string, error) {
	resp, _, err := s.larkcli.File.DownloadImage(ctx, &lark.DownloadImageReq{ImageKey: imageKey})
	if err != nil {
		return "", err
	}

	key, err := s.Upload(ctx, resp.File, fmt.Sprintf("%d@%s", time.Now().Nanosecond(), uuid.New()))
	if err != nil {
		return "", err
	}
	return s.PresignURL(ctx, key)
}

func (s *Service) BuildStringLarkMentions(ctx context.Context, mentions ...*lark.Mention) (string, error) {
	if len(mentions) == 0 {
		return "", nil
	}

	var _tmpl_lark_message_mentions = template.Must(template.New("lark_message_mentions_summary").
		Parse(`<mentions>{{ range .Mentions }}
- {{ .Name }}@{{ .ID }}
{{ end }}</mentions>
`))

	var b bytes.Buffer
	_ = _tmpl_lark_message_mentions.Execute(&b, struct{ Mentions []*lark.Mention }{Mentions: mentions})
	return b.String(), nil
}

func (s *Service) BuildStringLarkMessage(ctx context.Context, content *lark.MessageContent) (string, error) {
	var builder strings.Builder
	switch content.MsgType {
	case lark.MsgTypeText:
		_, _ = builder.WriteString(content.Text.Text)
	case lark.MsgTypeImage:
		url, err := s.BuildStringLarkImageURL(ctx, content.Image.ImageKey)
		if err != nil {
			return "", err
		}
		_, _ = builder.WriteString("![image](" + url + ")")
	case lark.MsgTypePost:
		for _, items := range content.Post.Content {
			for _, item := range items {
				switch t := item.(type) {
				case lark.MessageContentPostText:
					_, _ = builder.WriteString(t.Text)
				case lark.MessageContentPostLink:
					fmt.Fprintf(&builder, "[%s](%s)", t.Text, t.Href)
				case lark.MessageContentPostImage:
					url, err := s.BuildStringLarkImageURL(ctx, t.ImageKey)
					if err != nil {
						return "", err
					}
					_, _ = builder.WriteString("![image](" + url + ")")
				}
			}
		}
	case lark.MsgTypeInteractive:
		str := content.Text.Text
		var interactive struct {
			Title    string                          `json:"title"`
			Elements [][]larkcard.MessageCardElement `json:"elements"`
		}
		if err := json.Unmarshal([]byte(str), &interactive); err != nil {
			return "", err
		}
		_, _ = builder.WriteString("<card>\n")
		_, _ = builder.WriteString("<card_title>" + interactive.Title + "</card_title>\n")
		for _, elements := range interactive.Elements {
			_, _ = builder.WriteString("<card_section>\n")
			for _, element := range elements {
				switch t := element.(type) {
				case larkcard.MessageCardText:
					_, _ = builder.WriteString("<element>" + t.Text() + "</element>\n")
				}
			}
			_, _ = builder.WriteString("</card_section>\n")
		}
		_, _ = builder.WriteString("</card>\n")
	}
	return builder.String(), nil
}

func (s *Service) BuildStringLarkHistory(ctx context.Context, id *string, containerIdType lark.ContainerIDType,
) (string, error) {
	if id == nil {
		return "", nil
	}

	wstart, wend := "<chat_history>", "</chat_history>"
	switch containerIdType {
	case lark.ContainerIDTypeThread:
		wstart, wend = "<thread_history>", "</thread_history>"
	}

	historyBuilder := strings.Builder{}
	var _tmpl_list_messages = template.Must(template.New("lark_list_messages_summary").Funcs(template.FuncMap{
		"build_sender": func(sender *lark.Sender) string {
			var res string
			switch sender.SenderType {
			case "app":
				res, _ = s.BuildStringLarkApp(ctx, sender.ID, sender.IDType)
			case "user":
				res, _ = s.BuildStringLarkUser(ctx, sender.ID, sender.IDType)
			}
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
	}).Parse(wstart + `{{ range .Messages }}
[{{ .Timestamp.Format "2006-01-02 15:04:05" }}]
{{ if .Mentions }}{{ .Mentions | build_mentions }}{{ end }}{{ .Sender | build_sender }}: {{ .Content | build_content }}
{{ end }}` + wend))

	resp, _, err := s.larkcli.Message.GetMessageList(ctx, &lark.GetMessageListReq{
		ContainerIDType: containerIdType, ContainerID: *id,
		PageSize: new(int64(40)), SortType: new("ByCreateTimeDesc"),
	})
	if err != nil {
		return "", err
	}

	type Message struct {
		Timestamp time.Time
		Mentions  []*lark.Mention
		Sender    *lark.Sender
		Content   *lark.MessageContent
	}

	slices.Reverse(resp.Items)
	messages := make([]Message, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.Body == nil {
			continue
		}

		unixmilli, _ := strconv.ParseInt(item.CreateTime, 10, 64)
		message := Message{
			Mentions:  item.Mentions,
			Sender:    item.Sender,
			Timestamp: time.UnixMilli(int64(unixmilli)),
		}
		if item.Deleted {
			message.Content = &lark.MessageContent{
				Text:    &lark.MessageContentText{Text: "![message deleted]"},
				MsgType: lark.MsgTypeText,
			}
		} else {
			content, err := lark.UnwrapMessageContent(item.MsgType, item.Body.Content)
			if err != nil {
				if err.Error() == "unknown message type: interactive" {
					message.Content = &lark.MessageContent{
						MsgType: lark.MsgTypeInteractive,
						Text:    &lark.MessageContentText{Text: item.Body.Content},
					}
				} else {
					slog.ErrorContext(
						ctx, "failed to unwrap message content",
						"err", err, "type", item.MsgType, "content", item.Body.Content,
					)
				}
			}
			message.Content = content
		}
		messages = append(messages, message)
	}
	_ = _tmpl_list_messages.Execute(&historyBuilder, struct{ Messages []Message }{Messages: messages})
	return historyBuilder.String(), nil
}
