package chat

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	encryptedMessageType = "p2p-chat/message/v1"
	messageCipherName    = "AES-256-GCM"
)

var ErrInvalidEncryptedMessage = errors.New("invalid encrypted message")

type encryptedMessage struct {
	Type       string `json:"type"`
	Algorithm  string `json:"alg"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func sealMessage(message ChatMessage, roomKey string) ([]byte, error) {
	plaintext, err := EncodeChatMessage(message)
	if err != nil {
		return nil, err
	}
	aead, err := messageAEAD(message.Room, roomKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	envelope := encryptedMessage{
		Type:       encryptedMessageType,
		Algorithm:  messageCipherName,
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(aead.Seal(nil, nonce, plaintext, messageAAD(message.Room))),
	}
	return json.Marshal(envelope)
}

func openMessage(data []byte, room, roomKey string) (ChatMessage, error) {
	var envelope encryptedMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ChatMessage{}, err
	}
	if envelope.Type != encryptedMessageType || envelope.Algorithm != messageCipherName {
		return ChatMessage{}, ErrInvalidEncryptedMessage
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("%w: nonce", ErrInvalidEncryptedMessage)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("%w: ciphertext", ErrInvalidEncryptedMessage)
	}
	aead, err := messageAEAD(room, roomKey)
	if err != nil {
		return ChatMessage{}, err
	}
	if len(nonce) != aead.NonceSize() {
		return ChatMessage{}, fmt.Errorf("%w: nonce size", ErrInvalidEncryptedMessage)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, messageAAD(room))
	if err != nil {
		return ChatMessage{}, fmt.Errorf("%w: decrypt", ErrInvalidEncryptedMessage)
	}
	return DecodeChatMessage(plaintext)
}

func messageAEAD(room, roomKey string) (cipher.AEAD, error) {
	key, err := messageCipherKey(room, roomKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func messageCipherKey(room, roomKey string) ([]byte, error) {
	room = strings.TrimSpace(room)
	roomKey = strings.TrimSpace(roomKey)
	if err := ValidateRoom(room); err != nil {
		return nil, err
	}
	if err := ValidateRoomKey(roomKey); err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, []byte(roomKey))
	mac.Write([]byte("p2p-chat-message-v1"))
	mac.Write([]byte{0})
	mac.Write([]byte(room))
	return mac.Sum(nil), nil
}

func messageAAD(room string) []byte {
	return []byte(encryptedMessageType + "\x00" + room)
}
