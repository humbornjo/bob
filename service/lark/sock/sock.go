package larksock

import (
	"context"
	"fmt"
	"sync"

	"github.com/chyroc/lark"
)

type Socket interface {
	Close(context.Context, error) error
	Send(context.Context, ...lark.MessageContentPostItem) error
}

func NewSendSocket(larkcli *lark.Lark, chatID string) Socket {
	once, ch := sync.Once{}, make(chan error, 1)
	sock := &socket{larkcli: larkcli, chanErr: ch}

	sock.closure = func(ctx context.Context, content string) (bool, error) {
		done := false
		once.Do(func() {
			done = true
			defer close(ch)
			resp, _, err := larkcli.Message.Send().ToChatID(chatID).SendPost(ctx, content)
			if err != nil {
				ch <- err
				return
			}
			sock.messageID = resp.MessageID
		})
		return done, <-ch
	}
	return sock
}

func NewReplySocket(larkcli *lark.Lark, messageID string) Socket {
	once, chErr := sync.Once{}, make(chan error, 1)
	sock := &socket{larkcli: larkcli, chanErr: chErr}

	sock.closure = func(ctx context.Context, content string) (bool, error) {
		done := false
		once.Do(func() {
			done = true
			defer close(chErr)
			resp, _, err := larkcli.Message.Reply(messageID).SendPost(ctx, content)
			if err != nil {
				chErr <- err
				return
			}
			sock.messageID = resp.MessageID
		})
		return done, <-chErr
	}
	return sock
}

type socket struct {
	larkcli   *lark.Lark
	messageID string
	post      lark.MessageContentPost
	chanErr   <-chan error
	closure   func(context.Context, string) (bool, error)
}

func (s *socket) Close(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	text := fmt.Sprintf("**[ERROR]: %s**", err.Error())
	if err := s.send(ctx, &lark.MessageContentPostMD{Text: text}); err != nil {
		return err
	}
	return err
}

func (s *socket) Send(ctx context.Context, items ...lark.MessageContentPostItem) error {
	return s.send(ctx, items...)
}

func (s *socket) send(ctx context.Context, items ...lark.MessageContentPostItem) error {
	if len(items) == 0 {
		return nil
	}

	s.post.Content = append(s.post.Content, items)
	post := (&lark.MessageContentPostAll{ZhCn: &s.post}).String()
	if ok, err := s.closure(ctx, post); ok && err != nil {
		return err
	}
	if _, _, err := s.larkcli.Message.UpdateMessageEdit(ctx, &lark.UpdateMessageEditReq{
		Content: post, MessageID: s.messageID, MsgType: lark.MsgTypePost,
	}); err != nil {
		return err
	}
	return nil
}
