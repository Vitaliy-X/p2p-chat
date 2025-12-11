package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type SessionConfig struct {
	Room     string
	RoomKey  string
	Username string
	NoDHT    bool
	Peers    []ma.Multiaddr
	Relays   []ma.Multiaddr
	Store    Store
	Logger   *log.Logger
}

type SessionEvent struct {
	Type    string
	Message *ChatMessage
	PeerID  string
	Error   string
}

type PeerInfo struct {
	ID        string   `json:"id"`
	Addresses []string `json:"addresses"`
}

type Session struct {
	ctx    context.Context
	cancel context.CancelFunc

	h      host.Host
	topic  *pubsub.Topic
	sub    *pubsub.Subscription
	store  Store
	room   string
	user   User
	logger *log.Logger

	deduper *MessageDeduper
	events  chan SessionEvent

	mdnsService io.Closer

	mu     sync.RWMutex
	closed bool
}

func StartSession(ctx context.Context, cfg SessionConfig) (*Session, error) {
	if cfg.Store == nil {
		cfg.Store = NewNoopStore()
	}
	if cfg.Logger == nil {
		cfg.Logger = log.New(io.Discard, "", 0)
	}
	room := strings.TrimSpace(cfg.Room)
	if room == "" {
		room = DefaultTopicName
	}
	if err := ValidateRoom(room); err != nil {
		return nil, err
	}
	if err := ValidateRoomKey(cfg.RoomKey); err != nil {
		return nil, err
	}
	privateTopic, err := privateRoomTopic(room, cfg.RoomKey)
	if err != nil {
		return nil, err
	}
	if cfg.Username != "" {
		cfg.Username = strings.TrimSpace(cfg.Username)
		if err := ValidateUsername(cfg.Username); err != nil {
			return nil, err
		}
	}

	opts, err := hostOptions(cfg.Relays)
	if err != nil {
		return nil, fmt.Errorf("invalid relay address: %w", err)
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	s := &Session{
		ctx:     sessionCtx,
		cancel:  cancel,
		h:       h,
		store:   cfg.Store,
		room:    room,
		logger:  cfg.Logger,
		deduper: NewMessageDeduper(4096),
		events:  make(chan SessionEvent, 128),
	}
	s.user = User{ID: h.ID(), Username: cfg.Username}
	if s.user.Username == "" {
		s.user.Username = DefaultUserName(h.ID())
	}
	if err := ValidateUsername(s.user.Username); err != nil {
		s.Close()
		return nil, err
	}
	if err := saveRuntimeSettings(sessionCtx, s.store, s.room, s.user.Username); err != nil {
		cfg.Logger.Println("Settings warning:", err)
	}

	s.watchPeers()

	mdnsService, err := startMDNS(sessionCtx, h, privateMDNSServiceName(privateTopic), cfg.Logger, io.Discard)
	if err != nil {
		cfg.Logger.Println("mDNS warning:", err)
	} else {
		s.mdnsService = mdnsService
	}

	connectToConfiguredPeers(sessionCtx, h, cfg.Peers, cfg.Logger, io.Discard)
	if !cfg.NoDHT {
		go searchPeers(sessionCtx, h, privateTopic, cfg.Logger, io.Discard)
	}

	ps, err := pubsub.NewGossipSub(sessionCtx, h)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to create pubsub service: %w", err)
	}
	topic, err := ps.Join(privateTopic)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to join topic: %w", err)
	}
	s.topic = topic

	sub, err := topic.Subscribe()
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to subscribe to topic: %w", err)
	}
	s.sub = sub

	go s.receive()
	return s, nil
}

func (s *Session) Send(ctx context.Context, text string) (ChatMessage, error) {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return ChatMessage{}, errors.New("chat session is closed")
	}

	message, err := NewChatMessage(s.room, strings.TrimSpace(text), s.user)
	if err != nil {
		return ChatMessage{}, err
	}
	if _, err := s.store.SaveMessage(ctx, message); err != nil {
		s.logger.Println("Failed to save outgoing message:", err)
	}
	s.deduper.SeenOrAdd(message.ID)
	s.emit(SessionEvent{Type: "message", Message: &message})

	data, err := EncodeChatMessage(message)
	if err != nil {
		return ChatMessage{}, err
	}
	if err := s.topic.Publish(ctx, data); err != nil {
		return ChatMessage{}, err
	}
	return message, nil
}

func (s *Session) ConnectPeer(ctx context.Context, value string) error {
	addr, err := ma.NewMultiaddr(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return err
	}
	if info.ID == s.h.ID() {
		return nil
	}
	return s.h.Connect(ctx, *info)
}

func (s *Session) Events() <-chan SessionEvent {
	return s.events
}

func (s *Session) Done() <-chan struct{} {
	return s.ctx.Done()
}

func (s *Session) History(ctx context.Context, limit int) ([]ChatMessage, error) {
	return s.store.MessagesByRoom(ctx, s.room, limit)
}

func (s *Session) PeerInfo() PeerInfo {
	addrs := make([]string, 0, len(s.h.Addrs()))
	for _, addr := range s.h.Addrs() {
		addrs = append(addrs, fmt.Sprintf("%s/p2p/%s", addr, s.h.ID()))
	}
	return PeerInfo{
		ID:        s.h.ID().String(),
		Addresses: addrs,
	}
}

func (s *Session) PeerCount() int {
	count := 0
	for _, id := range s.h.Network().Peers() {
		if id != s.h.ID() && !isBootstrapPeer(id) {
			count++
		}
	}
	return count
}

func (s *Session) RoomPeerCount() int {
	if s.topic == nil {
		return 0
	}
	return len(s.topic.ListPeers())
}

func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	s.cancel()
	if s.sub != nil {
		s.sub.Cancel()
	}
	if s.mdnsService != nil {
		if err := s.mdnsService.Close(); err != nil {
			s.logger.Println("mDNS close warning:", err)
		}
	}
	if s.topic != nil {
		s.topic.Close()
	}
	if s.h != nil {
		return s.h.Close()
	}
	return nil
}

func (s *Session) receive() {
	for {
		message, err := s.sub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.logger.Println("Failed to read next message:", err)
			continue
		}
		chatMessage, err := unpackMessage(message.Data)
		if err != nil {
			s.logger.Println("Failed to decode message:", err)
			continue
		}
		if chatMessage.Room != s.room {
			continue
		}
		if s.deduper.SeenOrAdd(chatMessage.ID) {
			continue
		}
		if _, err := s.store.SaveMessage(s.ctx, chatMessage); err != nil {
			s.logger.Println("Failed to save incoming message:", err)
		}
		s.emit(SessionEvent{Type: "message", Message: &chatMessage})
	}
}

func (s *Session) watchPeers() {
	seen := newPeerSet()
	s.h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, conn network.Conn) {
			id := conn.RemotePeer()
			if id == s.h.ID() || isBootstrapPeer(id) {
				return
			}
			if seen.add(id) {
				s.emit(SessionEvent{Type: "peer_connected", PeerID: id.String()})
			}
		},
		DisconnectedF: func(_ network.Network, conn network.Conn) {
			id := conn.RemotePeer()
			if id == s.h.ID() || isBootstrapPeer(id) {
				return
			}
			if seen.remove(id) {
				s.emit(SessionEvent{Type: "peer_disconnected", PeerID: id.String()})
			}
		},
	})
}

func (s *Session) emit(event SessionEvent) {
	select {
	case s.events <- event:
	default:
		s.logger.Println("Chat event dropped:", event.Type)
	}
}
