package chat

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestSealOpenMessage(t *testing.T) {
	message := ChatMessage{
		ID:             "msg_secret",
		Room:           "room-1",
		Text:           "hello secret",
		SenderID:       mustDecodePeerID(t),
		SenderUsername: "alice",
		SentAt:         time.Now().UTC(),
		Version:        CurrentMessageVersion,
	}

	data, err := sealMessage(message, "shared-secret")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(message.Text)) {
		t.Fatal("encrypted payload must not contain plaintext")
	}
	if bytes.Contains(data, []byte(message.SenderUsername)) {
		t.Fatal("encrypted payload must not contain sender username")
	}

	got, err := openMessage(data, "room-1", "shared-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != message.Text || got.SenderUsername != message.SenderUsername {
		t.Fatalf("message = %#v, want %#v", got, message)
	}
}

func TestOpenMessageRejectsWrongKey(t *testing.T) {
	message := ChatMessage{
		ID:             "msg_secret",
		Room:           "room-1",
		Text:           "hello secret",
		SenderID:       mustDecodePeerID(t),
		SenderUsername: "alice",
		SentAt:         time.Now().UTC(),
		Version:        CurrentMessageVersion,
	}
	data, err := sealMessage(message, "shared-secret")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := openMessage(data, "room-1", "other-secret"); !errors.Is(err, ErrInvalidEncryptedMessage) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidEncryptedMessage)
	}
	if _, err := openMessage(data, "room-2", "shared-secret"); !errors.Is(err, ErrInvalidEncryptedMessage) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidEncryptedMessage)
	}
}
