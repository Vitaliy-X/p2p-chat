package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/libp2p/go-libp2p/core/peer"
	_ "modernc.org/sqlite"

	"p2p-chat/internal/chat"
)

const (
	driverName          = "sqlite"
	defaultHistoryLimit = 100
	maxSettingKeyLength = 128
	maxSettingValLength = 8192
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("sqlite db path is empty")
	}
	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
	}
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return err
		}
	}
	return s.db.PingContext(ctx)
}

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}

	for _, migration := range migrations {
		applied, err := migrationApplied(ctx, tx, migration.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		for _, statement := range migration.statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("migration %d: %w", migration.version, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			migration.version,
			nowString(),
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func migrationApplied(ctx context.Context, tx *sql.Tx, version int) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Store) SaveMessage(ctx context.Context, message chat.ChatMessage) (bool, error) {
	if err := message.Validate(); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO messages(
		id,
		room,
		text,
		sender_id,
		sender_username,
		sent_at,
		version,
		received_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO NOTHING`,
		message.ID,
		message.Room,
		message.Text,
		message.SenderID.String(),
		message.SenderUsername,
		message.SentAt.UTC().Format(time.RFC3339Nano),
		message.Version,
		nowString(),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) MessagesByRoom(ctx context.Context, room string, limit int) ([]chat.ChatMessage, error) {
	if err := chat.ValidateRoom(room); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultHistoryLimit
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		id,
		room,
		text,
		sender_id,
		sender_username,
		sent_at,
		version
	FROM (
		SELECT id, room, text, sender_id, sender_username, sent_at, version
		FROM messages
		WHERE room = ?
		ORDER BY sent_at DESC, id DESC
		LIMIT ?
	)
	ORDER BY sent_at ASC, id ASC`, room, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []chat.ChatMessage
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

type messageScanner interface {
	Scan(dest ...any) error
}

func scanMessage(scanner messageScanner) (chat.ChatMessage, error) {
	var message chat.ChatMessage
	var senderID string
	var sentAt string
	if err := scanner.Scan(
		&message.ID,
		&message.Room,
		&message.Text,
		&senderID,
		&message.SenderUsername,
		&sentAt,
		&message.Version,
	); err != nil {
		return chat.ChatMessage{}, err
	}

	id, err := peer.Decode(senderID)
	if err != nil {
		return chat.ChatMessage{}, err
	}
	message.SenderID = id

	parsedSentAt, err := time.Parse(time.RFC3339Nano, sentAt)
	if err != nil {
		return chat.ChatMessage{}, err
	}
	message.SentAt = parsedSentAt.UTC()

	if err := message.Validate(); err != nil {
		return chat.ChatMessage{}, err
	}
	return message, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	if err := validateSetting(key, value); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at)
	VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET
		value = excluded.value,
		updated_at = excluded.updated_at`,
		key,
		value,
		nowString(),
	)
	return err
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	if err := validateSettingKey(key); err != nil {
		return "", false, err
	}
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func validateSetting(key, value string) error {
	if err := validateSettingKey(key); err != nil {
		return err
	}
	if len(value) > maxSettingValLength || !utf8.ValidString(value) {
		return errors.New("invalid setting value")
	}
	return nil
}

func validateSettingKey(key string) error {
	if key == "" || len(key) > maxSettingKeyLength || !utf8.ValidString(key) {
		return errors.New("invalid setting key")
	}
	return nil
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
