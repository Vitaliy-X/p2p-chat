package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func Run(ctx context.Context, cfg Config) error {
	cfg = cfg.withDefaults()
	logger := log.New(cfg.Err, "", log.LstdFlags)
	if err := ValidateRoom(cfg.TopicName); err != nil {
		return err
	}
	if cfg.UserName != "" {
		if err := ValidateUsername(cfg.UserName); err != nil {
			return err
		}
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
	printHostInfo(cfg.Out, h)

	mdnsService, err := startMDNS(ctx, h, logger, cfg.Out)
	if err != nil {
		logger.Println("mDNS warning:", err)
	} else {
		defer mdnsService.Close()
	}

	connectToConfiguredPeers(ctx, h, cfg.Peers, logger, cfg.Out)
	if !cfg.NoDHT {
		go searchPeers(ctx, h, cfg.TopicName, logger, cfg.Out)
	}

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return fmt.Errorf("failed to create pubsub service: %w", err)
	}
	topic, err := ps.Join(cfg.TopicName)
	if err != nil {
		return fmt.Errorf("failed to join topic: %w", err)
	}
	defer topic.Close()

	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}
	defer sub.Cancel()

	go startChat(ctx, cfg.In, topic, cfg.TopicName, user, logger, cfg.Out)

	if err := receiveMessages(ctx, sub, NewMessageDeduper(4096), logger, cfg.Out); err != nil {
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

func startChat(ctx context.Context, in io.Reader, topic messagePublisher, room string, user User, logger *log.Logger, out io.Writer) {
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
		data, err := packMessage(room, message, user)
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

func receiveMessages(ctx context.Context, sub messageSubscription, deduper *MessageDeduper, logger *log.Logger, out io.Writer) error {
	for {
		message, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Println("Failed to read next message:", err)
			continue
		}
		chatMessage, err := unpackMessage(message.Data)
		if err != nil {
			logger.Println("Failed to decode message:", err)
			continue
		}
		if deduper.SeenOrAdd(chatMessage.ID) {
			continue
		}

		fmt.Fprintf(out, "%s: %s", chatMessage.SenderUsername, chatMessage.Text)
	}
}
