-- +goose Up
CREATE TABLE messages (
	id TEXT PRIMARY KEY,
	room TEXT NOT NULL,
	text TEXT NOT NULL,
	sender_id TEXT NOT NULL,
	sender_username TEXT NOT NULL,
	sent_at TEXT NOT NULL,
	version INTEGER NOT NULL,
	received_at TEXT NOT NULL
);

CREATE INDEX idx_messages_room_sent_at ON messages(room, sent_at, id);

CREATE TABLE settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE settings;
DROP INDEX idx_messages_room_sent_at;
DROP TABLE messages;
