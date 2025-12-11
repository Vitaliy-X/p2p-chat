package chat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	privateTopicPrefix = "/p2p-chat/room/v1/"
	mdnsServicePrefix  = "_p2p-chat-"

	minRoomKeyLength = 8
	maxRoomKeyLength = 256
)

var ErrInvalidRoomKey = errors.New("room key must be 8-256 UTF-8 bytes")

func ValidateRoomKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) < minRoomKeyLength || len(key) > maxRoomKeyLength || !utf8.ValidString(key) {
		return ErrInvalidRoomKey
	}
	return nil
}

func privateRoomTopic(room, key string) (string, error) {
	room = strings.TrimSpace(room)
	key = strings.TrimSpace(key)
	if err := ValidateRoom(room); err != nil {
		return "", err
	}
	if err := ValidateRoomKey(key); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("p2p-chat-room-v1"))
	mac.Write([]byte{0})
	mac.Write([]byte(room))
	sum := mac.Sum(nil)
	return privateTopicPrefix + base64.RawURLEncoding.EncodeToString(sum), nil
}

func privateMDNSServiceName(topic string) string {
	sum := sha256.Sum256([]byte(topic))
	return mdnsServicePrefix + hex.EncodeToString(sum[:12]) + "._udp"
}
