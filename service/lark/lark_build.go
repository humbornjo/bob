package larksvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chyroc/lark"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/mark3labs/mcp-go/mcp"
	anyllm "github.com/mozilla-ai/any-llm-go"

	"github.com/humbornjo/bob/package/llm"
	llmpersist "github.com/humbornjo/bob/package/llm/persist"
	llmskill "github.com/humbornjo/bob/package/llm/skill"
	llmtool "github.com/humbornjo/bob/package/llm/tool"
	larksock "github.com/humbornjo/bob/service/lark/sock"
	larktool "github.com/humbornjo/bob/service/lark/tool"
)

var (
	// ErrEmptyMentions
	ErrEmptyMentions = errors.New("empty mentions")
)

func (s *Service) BuildCompletion(ctx context.Context, messages []anyllm.Message, eMessage *larkim.EventMessage,
) (io.WriteCloser, anyllm.CompletionParams, error) {
	toolCreateMessageSend := larktool.NewCreateMessageSend()
	toolCreateMessageReply := larktool.NewCreateMessageReply()
	params := anyllm.CompletionParams{
		Model:           s.model,
		Messages:        messages,
		ToolChoice:      "required",
		ReasoningEffort: anyllm.ReasoningEffortNone,
		Tools:           []anyllm.Tool{toolCreateMessageSend.Tool(), toolCreateMessageReply.Tool()},
	}

	var socket io.WriteCloser
	completion, err := llm.Completion(ctx, s.provider, params)
	if err != nil {
		slog.ErrorContext(ctx, "failed to call chat completion", "err", err)
		return nil, anyllm.CompletionParams{}, err
	}

	if len(completion.Choices) == 0 || len(completion.Choices[0].Message.ToolCalls) == 0 {
		slog.ErrorContext(ctx, "no choices")
		socket = larksock.NewSendSocket(s.larkcli, s.oapicli, *eMessage.ChatId)
	} else {
		switch completion.Choices[0].Message.ToolCalls[0].Function.Name {
		case toolCreateMessageSend.Function().Name:
			socket = larksock.NewSendSocket(s.larkcli, s.oapicli, *eMessage.ChatId)
		case toolCreateMessageReply.Function().Name:
			socket = larksock.NewReplySocket(s.larkcli, s.oapicli, *eMessage.MessageId)
		}
	}

	params.Stream = true
	params.ToolChoice = "auto"
	params.ParallelToolCalls = new(true)
	params.ReasoningEffort = anyllm.ReasoningEffortMedium

	return socket, params, nil
}

func (s *Service) BuildToolset(ctx context.Context) ([]anyllm.Tool, llmtool.FunctionHandler, error) {
	xs := []llmtool.Toolx{
		larktool.NewListMessagesChat(s.larkcli),
		larktool.NewListMessagesThread(s.larkcli),
		llmskill.NewToolSkillView(),
		llmskill.NewToolSkillsList(),
	}

	for _, mcpcli := range s.mcpclis {
		resp, err := mcpcli.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			return nil, nil, err
		}

		for _, tool := range resp.Tools {
			x := llmtool.FromMCP(mcpcli, tool)
			xs = append(xs, x)
		}
	}

	slog.DebugContext(ctx, "build toolset", "#tools", len(xs))
	tools, handler := llmtool.Wizard(xs...)
	return tools, handler, nil
}

func (s *Service) BuildMessages(ctx context.Context, eMessage *larkim.EventMessage, eSender *larkim.EventSender,
) (complex struct {
	SystemMessage   anyllm.Message
	HistoryMessages []anyllm.Message
	UserMessage     anyllm.Message
	Messages        []anyllm.Message
}, metadata map[string]any, err error) {
	messages := []anyllm.Message{s.BuildSystemMessage(
		_SYSTEM_PROMPT_TMPL,
		struct{ Date string }{Date: time.Now().Format("2006-01-02")},
	)}
	if historyMessages, err := s.BuildHistoryMessages(ctx, eMessage, eSender); err != nil {
		return complex, nil, err
	} else {
		complex.HistoryMessages = historyMessages
		messages = append(messages, historyMessages...)
	}

	if userMessage, err := s.BuildUserMessage(ctx, eMessage, eSender); err != nil {
		return complex, nil, err
	} else {
		complex.UserMessage = userMessage
		messages = append(messages, userMessage)
	}

	metadata = make(map[string]any)
	if eMessage.ChatId != nil {
		metadata["chat_id"] = *eMessage.ChatId
	}
	if eMessage.ThreadId != nil {
		metadata["thread_id"] = *eMessage.ThreadId
	}

	messages, err = s.TidyMessages(ctx, messages)
	complex.Messages = messages
	return complex, metadata, err
}

func (s *Service) BuildSystemMessage(tmpl *template.Template, data any) anyllm.Message {
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, data)
	if err != nil {
		return anyllm.Message{Role: anyllm.RoleSystem, Content: _SYSTEM_PROMPT}
	}
	return anyllm.Message{Role: anyllm.RoleSystem, Content: buf.String()}
}

func (s *Service) BuildHistoryMessages(ctx context.Context, eMessage *larkim.EventMessage, eSender *larkim.EventSender,
) ([]anyllm.Message, error) {
	innerFn := func(ctx context.Context, containerIdType lark.ContainerIDType, id string) ([]anyllm.Message, error) {
		messages := make([]anyllm.Message, 0)
		resp, _, err := s.larkcli.Message.GetMessageList(ctx, &lark.GetMessageListReq{
			ContainerIDType: lark.ContainerIDTypeChat, ContainerID: id,
			PageSize: new(int64(40)), SortType: new("ByCreateTimeDesc"),
			EndTime: eMessage.CreateTime,
		})
		if err != nil {
			return nil, err
		}

		messageIDs := make([]string, 0, len(resp.Items))
		for _, item := range resp.Items {
			messageIDs = append(messageIDs, item.MessageID)
		}
		storedMessages, err := s.ps.ListMessages(ctx, SERVICE_NAME, id, llmpersist.WithFilterMessageIDs(messageIDs...))
		if err != nil {
			return nil, err
		}

		slices.Reverse(resp.Items)
		for _, item := range resp.Items {
			if item.CreateTime >= *eMessage.CreateTime {
				continue
			}
			if idx := slices.IndexFunc(storedMessages, func(msg llmpersist.Message) bool {
				return msg.GetID() == item.MessageID
			}); idx != -1 {
				messages = append(messages, storedMessages[idx].GetContent()...)
				continue
			}
			msg, err := s.BuildLarkMessage(ctx, containerIdType, item)
			if err != nil {
				slog.ErrorContext(ctx, "failed to build lark message", "err", err, "content", item.Body.Content)
				msg = anyllm.Message{Role: anyllm.RoleUser, Content: item.Body.Content}
			}
			messages = append(messages, msg)
		}
		return messages, nil
	}

	var messages []anyllm.Message

	if chatID := eMessage.ChatId; chatID != nil && *chatID != "" {
		messagesChat, err := innerFn(ctx, lark.ContainerIDTypeChat, *eMessage.ChatId)
		if err != nil {
			return nil, err
		}
		// Prepend chat profile if exists
		if profile, err := s.ps.GetProfile(ctx, SERVICE_NAME, *eMessage.ChatId); err == nil && profile != "" {
			messages = append(messages, anyllm.Message{
				Role:    anyllm.RoleUser,
				Content: "## Chat Profile\n\n" + profile,
			})
		}
		messages = append(messages, messagesChat...)
	}

	if threadId := eMessage.ThreadId; threadId != nil && *threadId != "" {
		messagesThread, err := innerFn(ctx, lark.ContainerIDTypeThread, *eMessage.ThreadId)
		if err != nil {
			return nil, err
		}
		messages = append(messages, messagesThread...)
	}

	return messages, nil
}

func (s *Service) BuildUserMessage(ctx context.Context, eMessage *larkim.EventMessage, eSender *larkim.EventSender,
) (anyllm.Message, error) {
	contentParts := []anyllm.ContentPart{}

	content, err := lark.UnwrapMessageContent(lark.MsgType(*eMessage.MessageType), *eMessage.Content)
	if err != nil {
		return anyllm.Message{}, err
	}

	unixmilli, err := strconv.ParseInt(*eMessage.CreateTime, 10, 64)
	if err != nil {
		unixmilli = time.Now().UnixMilli()
	}

	containerIdType := lark.ContainerIDTypeChat
	if eMessage.ThreadId != nil && *eMessage.ThreadId != "" {
		containerIdType = lark.ContainerIDTypeThread
	}

	sender := &lark.Sender{SenderType: *eSender.SenderType}
	switch {
	case eSender.SenderId.OpenId != nil:
		sender.IDType = lark.IDTypeOpenID
		sender.ID = *eSender.SenderId.OpenId
	case eSender.SenderId.UnionId != nil:
		sender.IDType = lark.IDTypeUnionID
		sender.ID = *eSender.SenderId.UnionId
	case eSender.SenderId.UserId != nil:
		sender.IDType = lark.IDTypeUserID
		sender.ID = *eSender.SenderId.UserId
	}
	parts, err := s.BuildLarkMessageContentParts(ctx, sender, containerIdType, content, time.UnixMilli(unixmilli))
	if err != nil {
		return anyllm.Message{}, err
	}
	contentParts = append(contentParts, parts...)

	mentions := make([]*lark.Mention, 0, len(eMessage.Mentions))
	for _, mention := range eMessage.Mentions {
		var id string
		var idtype lark.IDType
		switch {
		case *mention.Id.UserId != "":
			id, idtype = *mention.Id.UserId, lark.IDTypeUserID
		case *mention.Id.OpenId != "":
			id, idtype = *mention.Id.OpenId, lark.IDTypeOpenID
		case *mention.Id.UnionId != "":
			id, idtype = *mention.Id.UnionId, lark.IDTypeUnionID
		}
		mentions = append(mentions, &lark.Mention{Key: *mention.Key, ID: id, IDType: idtype, Name: *mention.Name})
	}
	if *eMessage.ChatType != string(lark.ChatModeP2P) &&
		!slices.ContainsFunc(mentions, func(mention *lark.Mention) bool { return mention.ID == s.openid }) {
		return anyllm.Message{}, errors.New("need not to respond")
	}
	mentionsStr, err := s.BuildLarkMentions(ctx, mentions...)
	if err != nil {
		if !errors.Is(err, ErrEmptyMentions) {
			return anyllm.Message{}, nil
		}
	} else {
		slog.DebugContext(ctx, "mentions", "mentions", mentionsStr)
		contentParts = append(contentParts, anyllm.ContentPart{Type: "text", Text: mentionsStr})
	}

	slog.DebugContext(ctx, "user message", "#content", len(contentParts), "content", contentParts)
	return anyllm.Message{Role: anyllm.RoleUser, Content: contentParts}, nil
}

func (s *Service) BuildLarkMessage(
	ctx context.Context,
	containerIdType lark.ContainerIDType, item *lark.GetMessageListRespItem,
) (message anyllm.Message, err error) {
	content := &lark.MessageContent{}
	if item.Deleted {
		content.MsgType = lark.MsgTypeText
		content.Text = &lark.MessageContentText{Text: "[message deleted]"}
	} else {
		content, err = lark.UnwrapMessageContent(item.MsgType, item.Body.Content)
		if err != nil {
			content = &lark.MessageContent{}
			if err.Error() == "unknown message type: interactive" {
				content.MsgType = lark.MsgTypeInteractive
				content.Text = &lark.MessageContentText{Text: item.Body.Content}
			} else {
				slog.ErrorContext(
					ctx, "failed to unwrap message content",
					"err", err, "type", item.MsgType, "content", item.Body.Content,
				)
				return anyllm.Message{}, err
			}
		}
	}

	unixmilli, err := strconv.ParseInt(item.CreateTime, 10, 64)
	if err != nil {
		unixmilli = time.Now().UnixMilli()
	}
	ts := time.UnixMilli(unixmilli)
	contentParts, err := s.BuildLarkMessageContentParts(ctx, item.Sender, containerIdType, content, ts)
	if err != nil {
		return anyllm.Message{}, err
	}

	mentionStr, err := s.BuildLarkMentions(ctx, item.Mentions...)
	if err != nil {
		if !errors.Is(err, ErrEmptyMentions) {
			return anyllm.Message{}, nil
		}
	} else {
		contentParts = append(contentParts, anyllm.ContentPart{Type: "text", Text: mentionStr})
	}

	var name string
	role := anyllm.RoleUser
	switch item.Sender.SenderType {
	case "app":
		if item.Sender.ID == s.appid {
			role = anyllm.RoleAssistant
		}
		_, name, _ = s.RetrieveLarkApp(ctx, item.Sender.ID, item.Sender.IDType)
	case "user":
		_, name, _ = s.RetrieveLarkUser(ctx, item.Sender.ID, item.Sender.IDType)
	}
	return anyllm.Message{Name: name, Role: role, Content: contentParts}, nil
}

func (s *Service) BuildLarkMessageContentParts(
	ctx context.Context,
	sender *lark.Sender, containIdType lark.ContainerIDType, content *lark.MessageContent, timestamp time.Time,
) ([]anyllm.ContentPart, error) {
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "#%s[%s]\n", timestamp.Format(time.RFC3339), containIdType)

	var senderID, senderName string
	switch sender.SenderType {
	case "app":
		senderID, senderName, _ = s.RetrieveLarkApp(ctx, sender.ID, sender.IDType)
	case "user":
		senderID, senderName, _ = s.RetrieveLarkUser(ctx, sender.ID, sender.IDType)
	}
	_, _ = fmt.Fprintf(&builder, "%s@%s: ", senderName, senderID)

	parts := make([]anyllm.ContentPart, 1)
	switch content.MsgType {
	case lark.MsgTypeText:
		_, _ = builder.WriteString(content.Text.Text)
	case lark.MsgTypeImage:
		url, err := s.RetrieveLarkImageURL(ctx, content.Image.ImageKey)
		if err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(&builder, "![image](%s)\n", url)
		parts = append(parts, anyllm.ContentPart{Type: "image_url", ImageURL: &anyllm.ImageURL{URL: url}})
	case lark.MsgTypePost:
		for _, items := range content.Post.Content {
			for _, item := range items {
				switch t := item.(type) {
				case lark.MessageContentPostText:
					_, _ = fmt.Fprint(&builder, t.Text)
				case lark.MessageContentPostLink:
					_, _ = fmt.Fprintf(&builder, " [%s](%s) ", t.Text, t.Href)
				case lark.MessageContentPostImage:
					url, err := s.RetrieveLarkImageURL(ctx, t.ImageKey)
					if err != nil {
						return nil, err
					}
					_, _ = fmt.Fprintf(&builder, " ![image](%s) ", url)
					parts = append(parts, anyllm.ContentPart{Type: "image_url", ImageURL: &anyllm.ImageURL{URL: url}})
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
			slog.ErrorContext(ctx, "failed to unmarshal interactive message", "err", err, "content", str)
			return nil, err
		}
		_, _ = fmt.Fprint(&builder, "\n\n<card>\n")
		_, _ = fmt.Fprintf(&builder, "<card_title>%s</card_title>\n", interactive.Title)
		for _, elements := range interactive.Elements {
			_, _ = fmt.Fprint(&builder, "<card_section>\n")
			for _, element := range elements {
				switch t := element.(type) {
				case larkcard.MessageCardText:
					_, _ = fmt.Fprint(&builder, "<element>"+t.Text()+"</element>\n")
				}
			}
			_, _ = fmt.Fprint(&builder, "</card_section>\n")
		}
		_, _ = fmt.Fprint(&builder, "</card>\n\n")
	}
	parts[0].Type, parts[0].Text = "text", builder.String()
	return parts, nil
}

func (s *Service) BuildLarkMentions(ctx context.Context, mentions ...*lark.Mention) (string, error) {
	if len(mentions) == 0 {
		return "", ErrEmptyMentions
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
