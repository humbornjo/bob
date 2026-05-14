package llmpersist

import (
	"context"
	"time"

	"github.com/google/uuid"
	anyllm "github.com/mozilla-ai/any-llm-go"
)

type Message interface {
	GetID() string
	GetContent() []anyllm.Message
}

type Option func(*config)

type config struct {
	Limit            int
	StartTime        time.Time
	EndTime          time.Time
	FilterMessageIDs []string
}

func WithLimit(limit int) Option {
	return func(cfg *config) {
		cfg.Limit = limit
	}
}

func WithStartTime(startTime time.Time) Option {
	return func(cfg *config) {
		cfg.StartTime = startTime
	}
}

func WithEndTime(endTime time.Time) Option {
	return func(cfg *config) {
		cfg.EndTime = endTime
	}
}

func WithFilterMessageIDs(messageIDs ...string) Option {
	return func(cfg *config) {
		cfg.FilterMessageIDs = messageIDs
	}
}

type Persistence interface {
	GetProfile(
		ctx context.Context,
		source string, id string) (string, error)

	UpdateProfile(
		ctx context.Context,
		source string, id string, profile string) error

	ListMessages(
		ctx context.Context,
		source string, chatID string, opts ...Option) ([]Message, error)

	SaveMessage(
		ctx context.Context,
		source string, chatID string, messageID string, messageContent []anyllm.Message) (uuid.UUID, error)
}
