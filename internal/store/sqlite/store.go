package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"p2p-chat/internal/chat"
)

const (
	driverName          = "sqlite"
	defaultHistoryLimit = 100
	maxSettingKeyLength = 128
	maxSettingValLength = 8192
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db *sql.DB

	mu       sync.RWMutex
	roomKeys map[string][]byte
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("sqlite db path is empty")
	}
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, err
	}
	store := &Store{
		db:       db,
		roomKeys: make(map[string][]byte),
	}
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

func ensureParentDir(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create sqlite directory: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA secure_delete = ON`,
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
	fsys, err := migrationFS()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		s.db,
		fsys,
		goose.WithTableName("goose_db_version"),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}

func migrationFS() (fs.FS, error) {
	return fs.Sub(migrations, "migrations")
}

func (s *Store) SaveMessage(ctx context.Context, message chat.ChatMessage) (bool, error) {
	if err := message.Validate(); err != nil {
		return false, err
	}

	nonce, ciphertext, err := s.sealMessage(message)
	if err != nil {
		return false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	roomID, _, err := ensureRoom(ctx, tx, message.Room)
	if err != nil {
		return false, err
	}

	result, err := tx.ExecContext(ctx, `INSERT INTO messages(
		id,
		room_id,
		sender_id,
		sent_at,
		version,
		received_at,
		encryption_version,
		nonce,
		ciphertext
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO NOTHING`,
		message.ID,
		roomID,
		message.SenderID.String(),
		message.SentAt.UTC().Format(time.RFC3339Nano),
		message.Version,
		nowString(),
		dbEncryptionVersion,
		nonce,
		ciphertext,
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func ensureRoom(ctx context.Context, tx *sql.Tx, room string) (int64, string, error) {
	salt, err := newRoomKeySalt()
	if err != nil {
		return 0, "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rooms(name, created_at, key_salt)
	VALUES (?, ?, ?)
	ON CONFLICT(name) DO NOTHING`,
		room,
		nowString(),
		salt,
	); err != nil {
		return 0, "", err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id, key_salt FROM rooms WHERE name = ?`, room).Scan(&id, &salt)
	if err != nil {
		return 0, "", err
	}
	return id, salt, nil
}

func (s *Store) SetRoomKey(ctx context.Context, room, key string) error {
	if err := chat.ValidateRoom(room); err != nil {
		return err
	}
	if err := chat.ValidateRoomKey(key); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, salt, err := ensureRoom(ctx, tx, room)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	derivedKey, err := deriveDBKey(room, key, salt)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.roomKeys[room] = derivedKey
	s.mu.Unlock()

	if err := s.verifyRoomKey(ctx, room); err != nil {
		s.mu.Lock()
		delete(s.roomKeys, room)
		s.mu.Unlock()
		return err
	}
	return nil
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
		sender_id,
		sent_at,
		version,
		encryption_version,
		nonce,
		ciphertext
	FROM (
		SELECT
			m.id,
			r.name AS room,
			m.sender_id,
			m.sent_at,
			m.version,
			m.encryption_version,
			m.nonce,
			m.ciphertext
		FROM messages m
		JOIN rooms r ON r.id = m.room_id
		WHERE r.name = ?
		ORDER BY m.sent_at DESC, m.id DESC
		LIMIT ?
	)
	ORDER BY sent_at ASC, id ASC`, room, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []chat.ChatMessage
	for rows.Next() {
		message, err := s.scanMessage(rows)
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

func (s *Store) scanMessage(scanner messageScanner) (chat.ChatMessage, error) {
	var message chat.ChatMessage
	var senderID string
	var sentAt string
	var encryptionVersion int
	var nonce sql.NullString
	var ciphertext sql.NullString
	if err := scanner.Scan(
		&message.ID,
		&message.Room,
		&senderID,
		&sentAt,
		&message.Version,
		&encryptionVersion,
		&nonce,
		&ciphertext,
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

	if encryptionVersion != dbEncryptionVersion {
		return chat.ChatMessage{}, fmt.Errorf("unsupported sqlite message encryption version %d", encryptionVersion)
	}
	if !nonce.Valid || !ciphertext.Valid {
		return chat.ChatMessage{}, errors.New("encrypted message payload is missing")
	}
	payload, err := s.openMessagePayload(message, nonce.String, ciphertext.String)
	if err != nil {
		return chat.ChatMessage{}, err
	}
	message.Text = payload.Text
	message.SenderUsername = payload.SenderUsername

	if err := message.Validate(); err != nil {
		return chat.ChatMessage{}, fmt.Errorf("stored message: %w", err)
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
