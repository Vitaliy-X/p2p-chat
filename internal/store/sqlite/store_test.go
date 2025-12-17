package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"p2p-chat/internal/chat"
)

func TestSaveMessageIdempotent(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()
	setRoomKey(t, ctx, store, "room-1")

	message := validMessage(t, "msg_one", "room-1", "hello\n", time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC))
	inserted, err := store.SaveMessage(ctx, message)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first SaveMessage should insert")
	}

	inserted, err = store.SaveMessage(ctx, message)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("second SaveMessage with same id should be idempotent")
	}

	history, err := store.MessagesByRoom(ctx, "room-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].ID != message.ID || history[0].Text != message.Text {
		t.Fatalf("history[0] = %#v, want %#v", history[0], message)
	}
}

func TestRoomHistory(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()
	setRoomKey(t, ctx, store, "room-1")
	setRoomKey(t, ctx, store, "room-2")

	messages := []chat.ChatMessage{
		validMessage(t, "msg_old", "room-1", "old\n", time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)),
		validMessage(t, "msg_mid", "room-1", "mid\n", time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC)),
		validMessage(t, "msg_new", "room-1", "new\n", time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)),
		validMessage(t, "msg_other", "room-2", "other\n", time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)),
	}
	for _, message := range messages {
		if _, err := store.SaveMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}

	history, err := store.MessagesByRoom(ctx, "room-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if history[0].ID != "msg_mid" || history[1].ID != "msg_new" {
		t.Fatalf("history ids = %s, %s; want msg_mid, msg_new", history[0].ID, history[1].ID)
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()

	_, ok, err := store.GetSetting(ctx, "last_room")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("missing setting should not be found")
	}

	if err := store.SetSetting(ctx, "last_room", "room-1"); err != nil {
		t.Fatal(err)
	}
	value, ok, err := store.GetSetting(ctx, "last_room")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "room-1" {
		t.Fatalf("setting = %q, %v; want room-1, true", value, ok)
	}

	if err := store.SetSetting(ctx, "last_room", "room-2"); err != nil {
		t.Fatal(err)
	}
	value, ok, err = store.GetSetting(ctx, "last_room")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "room-2" {
		t.Fatalf("updated setting = %q, %v; want room-2, true", value, ok)
	}
}

func TestMessagesAreEncrypted(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()
	setRoomKey(t, ctx, store, "room-1")

	message := validMessage(t, "msg_secret", "room-1", "secret text\n", time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC))
	if _, err := store.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}

	var encryptionVersion int
	var nonce, ciphertext string
	if err := store.db.QueryRowContext(ctx, `SELECT encryption_version, nonce, ciphertext
	FROM messages WHERE id = ?`, message.ID).Scan(
		&encryptionVersion,
		&nonce,
		&ciphertext,
	); err != nil {
		t.Fatal(err)
	}
	if encryptionVersion != dbEncryptionVersion {
		t.Fatalf("encryption version = %d, want %d", encryptionVersion, dbEncryptionVersion)
	}
	if nonce == "" || ciphertext == "" {
		t.Fatal("encrypted payload must be stored")
	}
	if ciphertext == message.Text || ciphertext == message.SenderUsername {
		t.Fatal("ciphertext must not equal plaintext")
	}
}

func TestMissingRoomKey(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()

	message := validMessage(t, "msg_no_key", "room-1", "hello\n", time.Now().UTC())
	if _, err := store.SaveMessage(ctx, message); !errors.Is(err, ErrMissingRoomKey) {
		t.Fatalf("SaveMessage() error = %v, want %v", err, ErrMissingRoomKey)
	}
	if _, err := store.MessagesByRoom(ctx, "room-1", 10); err != nil {
		t.Fatal(err)
	}
}

func TestRoomKeyVerification(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "chat.db")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	setRoomKey(t, ctx, store, "room-1")
	message := validMessage(t, "msg_secret_restart", "room-1", "secret text\n", time.Now().UTC())
	if _, err := store.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SetRoomKey(ctx, "room-1", "wrong-secret"); err == nil {
		t.Fatal("wrong room key should fail")
	}
	setRoomKey(t, ctx, store, "room-1")
	history, err := store.MessagesByRoom(ctx, "room-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Text != message.Text {
		t.Fatalf("history = %#v, want saved message", history)
	}
}

func TestOldSchemaMigrationDropsPlaintext(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "chat.db")
	createOldSchemaDB(t, ctx, path)

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if hasColumn(t, ctx, store.db, "messages", "text") {
		t.Fatal("messages.text should not exist after secure migration")
	}
	if hasColumn(t, ctx, store.db, "messages", "sender_username") {
		t.Fatal("messages.sender_username should not exist after secure migration")
	}

	var messageCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 0 {
		t.Fatalf("message count = %d, want 0", messageCount)
	}

	var roomCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rooms WHERE name = ?`, "room-1").Scan(&roomCount); err != nil {
		t.Fatal(err)
	}
	if roomCount != 1 {
		t.Fatalf("room count = %d, want 1", roomCount)
	}
}

func TestMigrateTwice(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "chat.db")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCreatesParentDir(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "chat.db")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSaveInvalidMessage(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()
	setRoomKey(t, ctx, store, "room-1")

	message := validMessage(t, "msg_invalid", "room-1", "hello\n", time.Now().UTC())
	message.Text = ""
	if _, err := store.SaveMessage(ctx, message); err == nil {
		t.Fatal("expected invalid message to fail")
	}
}

func openTempStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func setRoomKey(t *testing.T, ctx context.Context, store *Store, room string) {
	t.Helper()
	if err := store.SetRoomKey(ctx, room, "shared-secret"); err != nil {
		t.Fatal(err)
	}
}

func createOldSchemaDB(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	db, err := sql.Open(driverName, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	statements := []string{
		`CREATE TABLE goose_db_version (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version_id INTEGER NOT NULL,
			is_applied INTEGER NOT NULL,
			tstamp TIMESTAMP DEFAULT (datetime('now'))
		)`,
		`INSERT INTO goose_db_version(version_id, is_applied) VALUES (0, 1)`,
		`INSERT INTO goose_db_version(version_id, is_applied) VALUES (1, 1)`,
		`CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			room TEXT NOT NULL,
			text TEXT NOT NULL,
			sender_id TEXT NOT NULL,
			sender_username TEXT NOT NULL,
			sent_at TEXT NOT NULL,
			version INTEGER NOT NULL,
			received_at TEXT NOT NULL
		)`,
		`CREATE INDEX idx_messages_room_sent_at ON messages(room, sent_at, id)`,
		`CREATE TABLE settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO messages(
			id, room, text, sender_id, sender_username, sent_at, version, received_at
		) VALUES (
			'msg_old_plaintext',
			'room-1',
			'old plaintext',
			'12D3KooWNmwmLXo4RPrXToE2KiFCcEF5fGySSRGDRqt6y1so13up',
			'alice',
			'2026-06-17T10:00:00Z',
			1,
			'2026-06-17T10:00:01Z'
		)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
}

func hasColumn(t *testing.T, ctx context.Context, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func validMessage(t *testing.T, id, room, text string, sentAt time.Time) chat.ChatMessage {
	t.Helper()
	return chat.ChatMessage{
		ID:             id,
		Room:           room,
		Text:           text,
		SenderID:       mustPeerID(t),
		SenderUsername: "alice",
		SentAt:         sentAt.UTC(),
		Version:        chat.CurrentMessageVersion,
	}
}

func mustPeerID(t *testing.T) peer.ID {
	t.Helper()
	id, err := peer.Decode("12D3KooWNmwmLXo4RPrXToE2KiFCcEF5fGySSRGDRqt6y1so13up")
	if err != nil {
		t.Fatal(err)
	}
	return id
}
