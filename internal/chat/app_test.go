package chat

import (
	"bytes"
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

type fakePublisher struct {
	messages [][]byte
}

func (p *fakePublisher) Publish(_ context.Context, data []byte, _ ...pubsub.PubOpt) error {
	p.messages = append(p.messages, append([]byte(nil), data...))
	return nil
}

func TestStartChatPublishesInputLine(t *testing.T) {
	publisher := &fakePublisher{}
	user := User{ID: mustDecodePeerID(t), Username: "alice"}
	var out bytes.Buffer

	startChat(context.Background(), strings.NewReader("hello\n"), publisher, "general", user, discardLogger(), &out)

	if got, want := out.String(), "P2P chat launched\n"; got != want {
		t.Fatalf("out = %q, want %q", got, want)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages = %d, want 1", len(publisher.messages))
	}
	message, err := unpackMessage(publisher.messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if message.Room != "general" {
		t.Fatalf("room = %q, want %q", message.Room, "general")
	}
	if message.Text != "hello\n" {
		t.Fatalf("text = %q, want %q", message.Text, "hello\n")
	}
	if message.SenderUsername != "alice" {
		t.Fatalf("sender username = %q, want %q", message.SenderUsername, "alice")
	}
}

func TestStartChatDoesNotPublishPartialLineOnEOF(t *testing.T) {
	publisher := &fakePublisher{}
	user := User{ID: mustDecodePeerID(t), Username: "alice"}

	startChat(context.Background(), strings.NewReader("partial"), publisher, "general", user, discardLogger(), io.Discard)

	if len(publisher.messages) != 0 {
		t.Fatalf("published messages = %d, want 0", len(publisher.messages))
	}
}

func TestReceiveMessagesDeduplicatesByID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	message := testMessage(t, "msg_duplicate", "alice", "hello\n")
	data, err := EncodeChatMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	sub := &fakeSubscription{
		messages: [][]byte{data, data},
		cancel:   cancel,
	}
	var out bytes.Buffer

	err = receiveMessages(ctx, sub, NewMessageDeduper(8), discardLogger(), &out)
	if err != context.Canceled {
		t.Fatalf("receiveMessages() error = %v, want context.Canceled", err)
	}
	if got, want := out.String(), "alice: hello\n"; got != want {
		t.Fatalf("out = %q, want %q", got, want)
	}
}

type fakeSubscription struct {
	messages [][]byte
	cancel   context.CancelFunc
}

func (s *fakeSubscription) Next(ctx context.Context) (*pubsub.Message, error) {
	if len(s.messages) == 0 {
		s.cancel()
		return nil, ctx.Err()
	}
	data := s.messages[0]
	s.messages = s.messages[1:]
	return &pubsub.Message{Message: &pb.Message{Data: data}}, nil
}

func testMessage(t *testing.T, id, username, text string) ChatMessage {
	t.Helper()
	return ChatMessage{
		ID:             id,
		Room:           "general",
		Text:           text,
		SenderID:       mustDecodePeerID(t),
		SenderUsername: username,
		SentAt:         time.Now().UTC(),
		Version:        CurrentMessageVersion,
	}
}

func TestConfigWithDefaults(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.TopicName != DefaultTopicName {
		t.Fatalf("topic = %q, want %q", cfg.TopicName, DefaultTopicName)
	}
	if cfg.In == nil || cfg.Out == nil || cfg.Err == nil {
		t.Fatal("default streams must be set")
	}
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
