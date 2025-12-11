//go:build desktop || bindings

package main

import (
	"strings"
	"time"

	"p2p-chat/internal/chat"
)

type ConnectRequest struct {
	Username string `json:"username"`
	Room     string `json:"room"`
	RoomKey  string `json:"room_key"`
	DBPath   string `json:"db_path"`
	NoDHT    bool   `json:"no_dht"`
}

type State struct {
	Connected     bool          `json:"connected"`
	Status        string        `json:"status"`
	LastError     string        `json:"last_error,omitempty"`
	PeerCount     int           `json:"peer_count"`
	RoomPeerCount int           `json:"room_peer_count"`
	PeerInfo      chat.PeerInfo `json:"peer_info"`
	Messages      []MessageDTO  `json:"messages,omitempty"`
}

type MessageDTO struct {
	ID             string `json:"id"`
	Room           string `json:"room"`
	Text           string `json:"text"`
	SenderID       string `json:"sender_id"`
	SenderUsername string `json:"sender_username"`
	SentAt         string `json:"sent_at"`
	Version        int    `json:"version"`
}

func toMessageDTOs(messages []chat.ChatMessage) []MessageDTO {
	dtos := make([]MessageDTO, 0, len(messages))
	for _, message := range messages {
		dtos = append(dtos, toMessageDTO(message))
	}
	return dtos
}

func toMessageDTO(message chat.ChatMessage) MessageDTO {
	return MessageDTO{
		ID:             message.ID,
		Room:           message.Room,
		Text:           strings.TrimRight(message.Text, "\r\n"),
		SenderID:       message.SenderID.String(),
		SenderUsername: message.SenderUsername,
		SentAt:         message.SentAt.Local().Format(time.RFC3339),
		Version:        message.Version,
	}
}
