package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func Run(ctx context.Context, cfg Config) error {
	cfg = cfg.withDefaults()
	logger := log.New(cfg.Err, "", log.LstdFlags)
	if err := ValidateRoom(cfg.TopicName); err != nil {
		return err
	}
	if err := ValidateRoomKey(cfg.RoomKey); err != nil {
		return err
	}
	privateTopic, err := privateRoomTopic(cfg.TopicName, cfg.RoomKey)
	if err != nil {
		return err
	}
	if cfg.UserName != "" {
		if err := ValidateUsername(cfg.UserName); err != nil {
			return err
		}
	}
	if err := ConfigureStoreRoomKey(ctx, cfg.Store, cfg.TopicName, cfg.RoomKey); err != nil {
		return err
	}

	opts, err := hostOptions(cfg.Relays)
	if err != nil {
		return fmt.Errorf("invalid relay address: %w", err)
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}
	defer h.Close()

	user := User{ID: h.ID(), Username: cfg.UserName}
	if user.Username == "" {
		user.Username = DefaultUserName(h.ID())
	}
	if err := ValidateUsername(user.Username); err != nil {
		return err
	}
	if err := saveRuntimeSettings(ctx, cfg.Store, cfg.TopicName, user.Username); err != nil {
		logger.Println("Settings warning:", err)
	}
	printHostInfo(cfg.Out, h)

	mdnsService, err := startMDNS(ctx, h, privateMDNSServiceName(privateTopic), logger, cfg.Out)
	if err != nil {
		logger.Println("mDNS warning:", err)
	} else {
		defer mdnsService.Close()
	}

	connectToConfiguredPeers(ctx, h, cfg.Peers, logger, cfg.Out)
	if !cfg.NoDHT {
		go searchPeers(ctx, h, privateTopic, logger, cfg.Out)
	}

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return fmt.Errorf("failed to create pubsub service: %w", err)
	}
	topic, err := ps.Join(privateTopic)
	if err != nil {
		return fmt.Errorf("failed to join topic: %w", err)
	}
	defer topic.Close()

	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}
	defer sub.Cancel()

	deduper := NewMessageDeduper(4096)
	printRoomHistory(ctx, cfg.Store, cfg.TopicName, DefaultHistoryLimit, logger, cfg.Out)
	go startChat(ctx, cfg.In, topic, cfg.Store, deduper, cfg.TopicName, cfg.RoomKey, user, logger, cfg.Out)

	if err := receiveMessages(ctx, sub, cfg.Store, deduper, cfg.TopicName, cfg.RoomKey, logger, cfg.Out); err != nil {
		return err
	}
	return nil
}

type messagePublisher interface {
	Publish(context.Context, []byte, ...pubsub.PubOpt) error
}

type messageSubscription interface {
	Next(context.Context) (*pubsub.Message, error)
}

func startChat(ctx context.Context, in io.Reader, topic messagePublisher, store Store, deduper *MessageDeduper, room, roomKey string, user User, logger *log.Logger, out io.Writer) {
	fmt.Fprintln(out, "P2P chat launched")
	reader := bufio.NewReader(in)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		message, err := reader.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logger.Println("Failed to read input:", err)
			}
			return
		}
		chatMessage, err := NewChatMessage(room, message, user)
		if err != nil {
			logger.Println("Failed to create message:", err)
			continue
		}
		if _, err := store.SaveMessage(ctx, chatMessage); err != nil {
			logger.Println("Failed to save outgoing message:", err)
		}
		deduper.SeenOrAdd(chatMessage.ID)
		printChatLine(out, chatMessage.SenderUsername, chatMessage.Text)

		data, err := sealMessage(chatMessage, roomKey)
		if err != nil {
			logger.Println("Failed to encode message:", err)
			continue
		}
		if err := topic.Publish(ctx, data); err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Println("### Publish error:", err)
		}
	}
}

func receiveMessages(ctx context.Context, sub messageSubscription, store Store, deduper *MessageDeduper, room, roomKey string, logger *log.Logger, out io.Writer) error {
	for {
		message, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Println("Failed to read next message:", err)
			continue
		}
		chatMessage, err := openMessage(message.Data, room, roomKey)
		if err != nil {
			logger.Println("Failed to decrypt message:", err)
			continue
		}
		if chatMessage.Room != room {
			continue
		}
		if deduper.SeenOrAdd(chatMessage.ID) {
			continue
		}
		if _, err := store.SaveMessage(ctx, chatMessage); err != nil {
			logger.Println("Failed to save incoming message:", err)
		}

		printChatLine(out, chatMessage.SenderUsername, chatMessage.Text)
	}
}

func printRoomHistory(ctx context.Context, store Store, room string, limit int, logger *log.Logger, out io.Writer) {
	messages, err := store.MessagesByRoom(ctx, room, limit)
	if err != nil {
		logger.Println("History warning:", err)
		return
	}
	if len(messages) == 0 {
		return
	}
	fmt.Fprintf(out, "History (%s):\n", room)
	for _, message := range messages {
		printHistoryLine(out, message)
	}
}

func saveRuntimeSettings(ctx context.Context, store Store, room, username string) error {
	if err := store.SetSetting(ctx, "last_room", room); err != nil {
		return err
	}
	if err := store.SetSetting(ctx, "last_username", username); err != nil {
		return err
	}
	return nil
}

func printChatLine(out io.Writer, username, text string) {
	fmt.Fprintf(out, "%s: %s", username, text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(out)
	}
}

func printHistoryLine(out io.Writer, message ChatMessage) {
	fmt.Fprintf(out, "[%s] %s: %s",
		message.SentAt.Local().Format("2006-01-02 15:04:05"),
		message.SenderUsername,
		message.Text,
	)
	if !strings.HasSuffix(message.Text, "\n") {
		fmt.Fprintln(out)
	}
}
