package llmpersist

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/humbornjo/bob/package/sqltype"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	anyllm "github.com/mozilla-ai/any-llm-go"
)

var _ Persistence = (*persistenceDB)(nil)

type persistenceDB struct {
	db *sqlx.DB
}

func FromSqlx(db *sqlx.DB) (Persistence, error) {
	if _, err := db.Exec(_SCHEMA_MESSAGE_LOG); err != nil {
		return nil, err
	}
	if _, err := db.Exec(_SCHEMA_RELATION_CHAT_MESSAGE); err != nil {
		return nil, err
	}
	if _, err := db.Exec(_SCHEMA_PROFILE); err != nil {
		return nil, err
	}

	return &persistenceDB{db: db}, nil
}

var _SCHEMA_MESSAGE_LOG = `
CREATE TABLE IF NOT EXISTS message_log (
		id          UUID PRIMARY KEY,
		source      TEXT NOT NULL,
		message_id  TEXT NOT NULL,
		content     JSONB NOT NULL,
		created_at  TIMESTAMPTZ NOT NULL,
		updated_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_message_log_message_id
	ON message_log(message_id);`

type MessageLog struct {
	ID        uuid.UUID                      `db:"id"`
	Source    string                         `db:"source"`
	MessageID string                         `db:"message_id"`
	Content   sqltype.JSON[[]anyllm.Message] `db:"content"`
	CreatedAt time.Time                      `db:"created_at"`
	UpdatedAt time.Time                      `db:"updated_at"`
}

func (m *MessageLog) GetID() string {
	return m.MessageID
}

func (m *MessageLog) GetContent() []anyllm.Message {
	return m.Content.Unwrap()
}

var _SCHEMA_RELATION_CHAT_MESSAGE = `
CREATE TABLE IF NOT EXISTS relation_chat_message (
		source     TEXT NOT NULL,
		chat_id    TEXT NOT NULL,
		message_id TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL,
	  PRIMARY KEY (source, chat_id, message_id)
);
CREATE INDEX IF NOT EXISTS idx_relation_chat_message_created_at
	ON relation_chat_message(source, chat_id, created_at DESC);`

type RelationChatMessage struct {
	Source    string    `db:"source"`
	ChatID    string    `db:"chat_id"`
	MessageID string    `db:"message_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

var _SCHEMA_PROFILE = `
CREATE TABLE IF NOT EXISTS profile (
	source     TEXT NOT NULL,
	id         TEXT NOT NULL,
	content    TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (source, id)
);`

type Profile struct {
	Source    string    `db:"source"`
	ID        string    `db:"id"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (ins *persistenceDB) SaveMessage(
	ctx context.Context,
	source string, chatID string, messageID string, messageContent []anyllm.Message) (uuid.UUID, error) {
	tx, err := ins.db.BeginTxx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback() // nolint: errcheck

	id, now := uuid.New(), time.Now().UTC()
	content := sqltype.NewJSON(messageContent)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO message_log (id, source, message_id, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			content = EXCLUDED.content,
			updated_at = EXCLUDED.updated_at`, id, source, messageID, content, now, now)
	if err != nil {
		return uuid.Nil, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO relation_chat_message (source, chat_id, message_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (source, chat_id, message_id) DO UPDATE SET
			updated_at = EXCLUDED.updated_at
	`, source, chatID, messageID, now, now)
	if err != nil {
		return uuid.Nil, err
	}

	return id, tx.Commit()
}

func (ins *persistenceDB) ListMessages(ctx context.Context, source string, chatID string, opts ...Option,
) ([]Message, error) {
	cfg := &config{Limit: 100}
	for _, opt := range opts {
		opt(cfg)
	}

	query := `
		SELECT m.id, m.source, m.message_id, m.content, m.created_at, m.updated_at
		FROM message_log m
		JOIN relation_chat_message rcm ON m.message_id = rcm.message_id
		WHERE rcm.source = $1 AND rcm.chat_id = $2
	`
	args := []any{source, chatID}
	argIdx := 2

	if len(cfg.FilterMessageIDs) > 0 {
		argIdx++
		query += fmt.Sprintf(" AND m.message_id = ANY($%d)", argIdx)
		args = append(args, cfg.FilterMessageIDs)
	}

	if !cfg.StartTime.IsZero() {
		argIdx++
		query += fmt.Sprintf(" AND rcm.created_at >= $%d", argIdx)
		args = append(args, cfg.StartTime)
	}

	if !cfg.EndTime.IsZero() {
		argIdx++
		query += fmt.Sprintf(" AND rcm.created_at <= $%d", argIdx)
		args = append(args, cfg.EndTime)
	}

	argIdx++
	query += fmt.Sprintf(" ORDER BY rcm.created_at DESC LIMIT $%d", argIdx)
	args = append(args, cfg.Limit)

	var logs []MessageLog
	err := ins.db.SelectContext(ctx, &logs, query, args...)
	if err != nil {
		return nil, err
	}

	result := make([]Message, len(logs))
	for i := range logs {
		result[i] = &logs[i]
	}
	return result, nil
}

func (ins *persistenceDB) GetProfile(ctx context.Context, source string, id string) (string, error) {
	var profile Profile
	err := ins.db.GetContext(ctx, &profile, `
		SELECT source, id, content, created_at, updated_at
		FROM profile
		WHERE source = $1 AND id = $2
	`, source, id)
	if err != nil {
		return "", err
	}
	return profile.Content, nil
}

func (ins *persistenceDB) UpdateProfile(ctx context.Context, source string, id string, profile string) error {
	now := time.Now().UTC()
	_, err := ins.db.ExecContext(ctx, `
		INSERT INTO profile (source, id, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (source, id) DO UPDATE SET
			content = EXCLUDED.content,
			updated_at = EXCLUDED.updated_at
	`, source, id, profile, now, now)
	return err
}
