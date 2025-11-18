package chat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	CurrentMessageVersion = 1

	maxMessageIDLength = 128
	maxRoomLength      = 64
	maxTextLength      = 4096
	maxUsernameLength  = 64
)

var (
	ErrInvalidMessageID = errors.New("invalid message id")
	ErrInvalidRoom      = errors.New("invalid room")
	ErrInvalidText      = errors.New("invalid text")
	ErrInvalidSenderID  = errors.New("invalid sender id")
	ErrInvalidUsername  = errors.New("invalid username")
	ErrInvalidSentAt    = errors.New("invalid sent_at")
	ErrInvalidVersion   = errors.New("invalid message version")
)

type ChatMessage struct {
	ID             string    `json:"id"`
	Room           string    `json:"room"`
	Text           string    `json:"text"`
	SenderID       peer.ID   `json:"sender_id"`
	SenderUsername string    `json:"sender_username"`
	SentAt         time.Time `json:"sent_at"`
	Version        int       `json:"version"`
}

func NewChatMessage(room, text string, user User) (ChatMessage, error) {
	id, err := newMessageID()
	if err != nil {
		return ChatMessage{}, err
	}
	message := ChatMessage{
		ID:             id,
		Room:           strings.TrimSpace(room),
		Text:           text,
		SenderID:       user.ID,
		SenderUsername: strings.TrimSpace(user.Username),
		SentAt:         time.Now().UTC(),
		Version:        CurrentMessageVersion,
	}
	return message, message.Validate()
}

func (m ChatMessage) Validate() error {
	if err := ValidateMessageID(m.ID); err != nil {
		return err
	}
	if err := ValidateRoom(m.Room); err != nil {
		return err
	}
	if err := ValidateText(m.Text); err != nil {
		return err
	}
	if m.SenderID == "" {
		return ErrInvalidSenderID
	}
	if err := ValidateUsername(m.SenderUsername); err != nil {
		return err
	}
	if m.SentAt.IsZero() {
		return ErrInvalidSentAt
	}
	if m.Version != CurrentMessageVersion {
		return fmt.Errorf("%w: %d", ErrInvalidVersion, m.Version)
	}
	return nil
}

func ValidateMessageID(id string) error {
	if id == "" || id != strings.TrimSpace(id) || len(id) > maxMessageIDLength || !utf8.ValidString(id) {
		return ErrInvalidMessageID
	}
	for _, r := range id {
		if !isTokenRune(r) {
			return ErrInvalidMessageID
		}
	}
	return nil
}

func ValidateRoom(room string) error {
	if room == "" || room != strings.TrimSpace(room) || len(room) > maxRoomLength || !utf8.ValidString(room) {
		return ErrInvalidRoom
	}
	for _, r := range room {
		if !isTokenRune(r) {
			return ErrInvalidRoom
		}
	}
	return nil
}

func ValidateUsername(username string) error {
	if username == "" || username != strings.TrimSpace(username) || len(username) > maxUsernameLength || !utf8.ValidString(username) {
		return ErrInvalidUsername
	}
	for _, r := range username {
		if !isUsernameRune(r) {
			return ErrInvalidUsername
		}
	}
	return nil
}

func ValidateText(text string) error {
	if text == "" || len(text) > maxTextLength || !utf8.ValidString(text) {
		return ErrInvalidText
	}
	if strings.TrimSpace(text) == "" {
		return ErrInvalidText
	}
	for _, r := range text {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return ErrInvalidText
		}
	}
	return nil
}

func EncodeChatMessage(message ChatMessage) ([]byte, error) {
	if err := message.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(message)
}

func DecodeChatMessage(data []byte) (ChatMessage, error) {
	var message ChatMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return ChatMessage{}, err
	}
	message.ID = strings.TrimSpace(message.ID)
	message.Room = strings.TrimSpace(message.Room)
	message.SenderUsername = strings.TrimSpace(message.SenderUsername)
	message.SentAt = message.SentAt.UTC()
	if err := message.Validate(); err != nil {
		return ChatMessage{}, err
	}
	return message, nil
}

func packMessage(room, text string, user User) ([]byte, error) {
	message, err := NewChatMessage(room, text, user)
	if err != nil {
		return nil, err
	}
	return EncodeChatMessage(message)
}

func unpackMessage(data []byte) (ChatMessage, error) {
	return DecodeChatMessage(data)
}

func newMessageID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "msg_" + hex.EncodeToString(b[:]), nil
}

func isTokenRune(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '_' || r == '-' || r == '.'
}

func isUsernameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
}
