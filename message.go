package main

import "github.com/libp2p/go-libp2p/core/peer"

type ChatMessage struct {
	Message        string  `json:"message"`
	SenderID       peer.ID `json:"sender_id"`
	SenderUsername string  `json:"sender_username"`
}
