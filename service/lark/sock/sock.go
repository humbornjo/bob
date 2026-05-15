package larksock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chyroc/lark"
	larkoapi "github.com/larksuite/oapi-sdk-go/v3"
	larkcardkit "github.com/larksuite/oapi-sdk-go/v3/service/cardkit/v1"
)

var _PATTERNS = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}

var _ io.WriteCloser = (*socket)(nil)

func NewSendSocket(larkcli *lark.Lark, oapicli *larkoapi.Client, chatID string) io.WriteCloser {
	return _NewSocket(larkcli, oapicli, func(ctx context.Context, content string) (string, error) {
		resp, _, err := larkcli.Message.Send().ToChatID(chatID).SendCard(ctx, content)
		if err != nil {
			return "", err
		}
		return resp.MessageID, nil
	})
}

func NewReplySocket(larkcli *lark.Lark, oapicli *larkoapi.Client, messageID string) io.WriteCloser {
	return _NewSocket(larkcli, oapicli, func(ctx context.Context, card string) (string, error) {
		resp, _, err := larkcli.Message.Reply(messageID).SendCard(ctx, card)
		if err != nil {
			return "", err
		}
		return resp.MessageID, nil
	})
}

func NewP2pSocket(larkcli *lark.Lark, oapicli *larkoapi.Client, userID string) io.WriteCloser {
	return _NewSocket(larkcli, oapicli, func(ctx context.Context, card string) (string, error) {
		resp, _, err := larkcli.Message.Send().ToUserID(userID).SendCard(ctx, card)
		if err != nil {
			return "", err
		}
		return resp.MessageID, nil
	})
}

func _NewSocket(
	larkcli *lark.Lark, oapicli *larkoapi.Client,
	sendf func(ctx context.Context, card string) (string, error),
) io.WriteCloser {
	chErr, chContent := make(chan error, 1), make(chan string, 16)
	sock := &socket{larkcli: larkcli, oapicli: oapicli, chanErr: chErr, chanContent: chContent}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := sock.CreateCard(ctx); err != nil {
			chErr <- err
			return
		}

		card := map[string]any{"type": "card", "data": map[string]string{"card_id": sock.cardID}}
		jsonb, err := json.Marshal(card)
		if err != nil {
			chErr <- err
			return
		}
		sock.messageID, err = sendf(ctx, string(jsonb))
		if err != nil {
			chErr <- err
			return
		}

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			sock.seq++
			select {
			case content := <-sock.chanContent:
				sock.builder.WriteString(content)
				if err := sock.UpdateCardElement(ctx, sock.elementID, sock.builder.String()); err != nil {
					_, _ = fmt.Fprint(sock, err.Error())
					return
				}
				for content := range sock.chanContent {
					sock.seq++
					sock.builder.WriteString(content)
					if err := sock.UpdateCardElement(ctx, sock.elementID, sock.builder.String()); err != nil {
						_, _ = fmt.Fprint(sock, err.Error())
						return
					}
				}
				return
			case err := <-sock.chanErr:
				_, _ = fmt.Fprint(sock, err.Error())
				return
			default:
				if err := sock.UpdateCardElement(ctx, sock.elementID, _PATTERNS[sock.seq%len(_PATTERNS)]); err != nil {
					_, _ = fmt.Fprint(sock, err.Error())
					return
				}
			}
		}
	}()

	return sock
}

type socket struct {
	larkcli *lark.Lark
	oapicli *larkoapi.Client

	seq       int
	cardID    string
	elementID string

	builder     strings.Builder
	messageID   string
	chanErr     chan error
	chanContent chan string
}

func (s *socket) Write(p []byte) (int, error) {
	s.chanContent <- string(p)
	return len(p), nil
}

func (s *socket) Close() error {
	close(s.chanContent)
	return nil
}

func (s *socket) CreateCard(ctx context.Context) error {
	data := map[string]any{
		"schema": "2.0",
		"config": map[string]any{
			"streaming_mode": true,
		},
		"body": map[string]any{
			"elements": []map[string]any{
				{
					"element_id": "main",
					"tag":        "markdown",
					"content":    _PATTERNS[s.seq%len(_PATTERNS)],
				},
			},
		},
	}

	jsonb, _ := json.Marshal(data)
	fmt.Println(string(jsonb))
	resp, err := s.oapicli.Cardkit.V1.Card.Create(ctx,
		larkcardkit.NewCreateCardReqBuilder().
			Body(larkcardkit.NewCreateCardReqBodyBuilder().
				Type("card_json").Data(string(jsonb)).Build()).Build())
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("[%d] %s", resp.Code, resp.Msg)
	}
	s.elementID, s.cardID = "main", *resp.Data.CardId
	return nil
}

func (s *socket) UpdateCardElement(ctx context.Context, elementID string, content string) error {
	req := larkcardkit.NewContentCardElementReqBuilder().
		CardId(s.cardID).ElementId(elementID).
		Body(larkcardkit.NewContentCardElementReqBodyBuilder().
			Content(content).
			Sequence(s.seq).Build()).Build()
	resp, err := s.oapicli.Cardkit.V1.CardElement.Content(ctx, req)
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("[%d] %s", resp.StatusCode, resp.Msg)
	}
	return nil
}
