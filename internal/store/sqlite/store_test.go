package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"p2p-chat/internal/chat"
)

func TestStoreSaveMessageIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()

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

func TestStoreMessagesByRoomReturnsLatestMessagesInChronologicalOrder(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()

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

func TestStoreSettings(t *testing.T) {
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

func TestStoreMigrationsAreIdempotent(t *testing.T) {
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

func TestStoreRejectsInvalidMessage(t *testing.T) {
	ctx := context.Background()
	store := openTempStore(t, ctx)
	defer store.Close()

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
