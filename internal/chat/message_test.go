package chat

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const testPeerID = "12D3KooWNmwmLXo4RPrXToE2KiFCcEF5fGySSRGDRqt6y1so13up"

func mustDecodePeerID(t *testing.T) peer.ID {
	t.Helper()
	id, err := peer.Decode(testPeerID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPackMessage(t *testing.T) {
	id := mustDecodePeerID(t)
	data, err := packMessage("general", "hello\n", User{ID: id, Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	message, err := unpackMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	if message.ID == "" {
		t.Fatal("message id must be set")
	}
	if message.Room != "general" {
		t.Fatalf("room = %q, want %q", message.Room, "general")
	}
	if message.Text != "hello\n" {
		t.Fatalf("text = %q, want %q", message.Text, "hello\n")
	}
	if message.SenderID != id {
		t.Fatalf("sender id = %s, want %s", message.SenderID, id)
	}
	if message.SenderUsername != "alice" {
		t.Fatalf("sender username = %q, want %q", message.SenderUsername, "alice")
	}
	if message.SentAt.IsZero() {
		t.Fatal("sent_at must be set")
	}
	if message.Version != CurrentMessageVersion {
		t.Fatalf("version = %d, want %d", message.Version, CurrentMessageVersion)
	}
}

func TestUnpackMessageRejectsInvalidJSON(t *testing.T) {
	if _, err := unpackMessage([]byte("{")); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func TestEncodeDecodeChatMessage(t *testing.T) {
	id := mustDecodePeerID(t)
	sentAt := time.Date(2026, 6, 15, 12, 30, 0, 0, time.FixedZone("test", 3*60*60))
	message := ChatMessage{
		ID:             "msg_0123456789abcdef",
		Room:           "room-1",
		Text:           "hello",
		SenderID:       id,
		SenderUsername: "alice",
		SentAt:         sentAt,
		Version:        CurrentMessageVersion,
	}

	data, err := EncodeChatMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeChatMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != message.ID {
		t.Fatalf("id = %q, want %q", got.ID, message.ID)
	}
	if got.Room != message.Room {
		t.Fatalf("room = %q, want %q", got.Room, message.Room)
	}
	if got.Text != message.Text {
		t.Fatalf("text = %q, want %q", got.Text, message.Text)
	}
	if got.SenderUsername != "alice" {
		t.Fatalf("username = %q, want username", got.SenderUsername)
	}
	if !got.SentAt.Equal(sentAt) {
		t.Fatalf("sent_at = %s, want %s", got.SentAt, sentAt)
	}
}

func TestDecodeNormalizesStrings(t *testing.T) {
	id := mustDecodePeerID(t)
	raw, err := json.Marshal(ChatMessage{
		ID:             " msg_trimmed ",
		Room:           " general ",
		Text:           "hello",
		SenderID:       id,
		SenderUsername: " alice ",
		SentAt:         time.Now().UTC(),
		Version:        CurrentMessageVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	message, err := DecodeChatMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if message.ID != "msg_trimmed" || message.Room != "general" || message.SenderUsername != "alice" {
		t.Fatalf("message was not normalized: %#v", message)
	}
}

func TestChatMessageValidation(t *testing.T) {
	id := mustDecodePeerID(t)
	valid := ChatMessage{
		ID:             "msg_valid-1",
		Room:           "room_1",
		Text:           "hello",
		SenderID:       id,
		SenderUsername: "alice",
		SentAt:         time.Now().UTC(),
		Version:        CurrentMessageVersion,
	}

	tests := []struct {
		name string
		edit func(*ChatMessage)
		want error
	}{
		{name: "valid", edit: func(*ChatMessage) {}, want: nil},
		{name: "unicode username", edit: func(m *ChatMessage) { m.SenderUsername = "алиса-1" }, want: nil},
		{name: "empty id", edit: func(m *ChatMessage) { m.ID = "" }, want: ErrInvalidMessageID},
		{name: "bad id rune", edit: func(m *ChatMessage) { m.ID = "bad/id" }, want: ErrInvalidMessageID},
		{name: "empty room", edit: func(m *ChatMessage) { m.Room = "" }, want: ErrInvalidRoom},
		{name: "bad room rune", edit: func(m *ChatMessage) { m.Room = "bad room" }, want: ErrInvalidRoom},
		{name: "space padded room", edit: func(m *ChatMessage) { m.Room = " room_1 " }, want: ErrInvalidRoom},
		{name: "blank text", edit: func(m *ChatMessage) { m.Text = " \n\t" }, want: ErrInvalidText},
		{name: "control text", edit: func(m *ChatMessage) { m.Text = "hello\x00" }, want: ErrInvalidText},
		{name: "missing sender id", edit: func(m *ChatMessage) { m.SenderID = "" }, want: ErrInvalidSenderID},
		{name: "empty username", edit: func(m *ChatMessage) { m.SenderUsername = "" }, want: ErrInvalidUsername},
		{name: "space padded username", edit: func(m *ChatMessage) { m.SenderUsername = " alice " }, want: ErrInvalidUsername},
		{name: "bad username rune", edit: func(m *ChatMessage) { m.SenderUsername = "alice bob" }, want: ErrInvalidUsername},
		{name: "zero sent at", edit: func(m *ChatMessage) { m.SentAt = time.Time{} }, want: ErrInvalidSentAt},
		{name: "bad version", edit: func(m *ChatMessage) { m.Version = 2 }, want: ErrInvalidVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := valid
			tt.edit(&message)
			err := message.Validate()
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}
