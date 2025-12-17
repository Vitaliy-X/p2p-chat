package sqlite

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"p2p-chat/internal/chat"
)

const dbEncryptionVersion = 1

const (
	dbKeySaltSize = 16
	dbKeySize     = 32
)

var ErrMissingRoomKey = errors.New("sqlite room key is not configured")

type messagePayload struct {
	Text           string `json:"text"`
	SenderUsername string `json:"sender_username"`
}

func newRoomKeySalt() (string, error) {
	salt := make([]byte, dbKeySaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt), nil
}

func deriveDBKey(room, key, saltText string) ([]byte, error) {
	room = strings.TrimSpace(room)
	key = strings.TrimSpace(key)
	if err := chat.ValidateRoom(room); err != nil {
		return nil, err
	}
	if err := chat.ValidateRoomKey(key); err != nil {
		return nil, err
	}
	salt, err := hex.DecodeString(saltText)
	if err != nil || len(salt) != dbKeySaltSize {
		return nil, errors.New("invalid sqlite room key salt")
	}
	saltMAC := hmac.New(sha256.New, salt)
	saltMAC.Write([]byte("p2p-chat-sqlite-message-v1"))
	saltMAC.Write([]byte{0})
	saltMAC.Write([]byte(room))
	kdfSalt := saltMAC.Sum(nil)
	return argon2.IDKey([]byte(key), kdfSalt, 1, 64*1024, 1, dbKeySize), nil
}

func (s *Store) sealMessage(message chat.ChatMessage) (string, string, error) {
	aead, err := s.messageAEAD(message.Room)
	if err != nil {
		return "", "", err
	}
	plaintext, err := json.Marshal(messagePayload{
		Text:           message.Text,
		SenderUsername: message.SenderUsername,
	})
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, messageAAD(message))
	return base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func (s *Store) openMessagePayload(message chat.ChatMessage, nonceText, ciphertextText string) (messagePayload, error) {
	aead, err := s.messageAEAD(message.Room)
	if err != nil {
		return messagePayload{}, err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(nonceText)
	if err != nil {
		return messagePayload{}, err
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(ciphertextText)
	if err != nil {
		return messagePayload{}, err
	}
	if len(nonce) != aead.NonceSize() {
		return messagePayload{}, errors.New("invalid sqlite message nonce")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, messageAAD(message))
	if err != nil {
		return messagePayload{}, err
	}
	var payload messagePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return messagePayload{}, err
	}
	if err := chat.ValidateText(payload.Text); err != nil {
		return messagePayload{}, err
	}
	if err := chat.ValidateUsername(payload.SenderUsername); err != nil {
		return messagePayload{}, err
	}
	return payload, nil
}

func (s *Store) messageAEAD(room string) (cipher.AEAD, error) {
	key, ok := s.roomKey(room)
	if !ok {
		return nil, fmt.Errorf("%w for room %q", ErrMissingRoomKey, room)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Store) roomKey(room string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.roomKeys[room]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), key...), true
}

func messageAAD(message chat.ChatMessage) []byte {
	return []byte(fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d",
		message.ID,
		message.Room,
		message.SenderID.String(),
		message.SentAt.UTC().Format(time.RFC3339Nano),
		message.Version,
	))
}

func (s *Store) verifyRoomKey(ctx context.Context, room string) error {
	row := s.db.QueryRowContext(ctx, `SELECT
		m.id,
		r.name,
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
	LIMIT 1`, room)
	if _, err := s.scanMessage(row); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("verify sqlite room key: %w", err)
	}
	return nil
}
