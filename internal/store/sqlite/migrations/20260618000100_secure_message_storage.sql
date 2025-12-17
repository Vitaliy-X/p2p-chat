-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;
PRAGMA secure_delete = ON;

CREATE TABLE rooms (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	key_salt TEXT NOT NULL CHECK (key_salt <> '')
);

INSERT INTO rooms(name, created_at, key_salt)
SELECT room, MIN(received_at), lower(hex(randomblob(16)))
FROM messages
GROUP BY room
ORDER BY room;

CREATE TABLE messages_secure (
	id TEXT PRIMARY KEY,
	room_id INTEGER NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
	sender_id TEXT NOT NULL,
	sent_at TEXT NOT NULL,
	version INTEGER NOT NULL,
	received_at TEXT NOT NULL,
	encryption_version INTEGER NOT NULL DEFAULT 1 CHECK (encryption_version > 0),
	nonce TEXT NOT NULL CHECK (nonce <> ''),
	ciphertext TEXT NOT NULL CHECK (ciphertext <> '')
);

DROP INDEX idx_messages_room_sent_at;
DROP TABLE messages;
ALTER TABLE messages_secure RENAME TO messages;

CREATE INDEX idx_messages_room_sent_at ON messages(room_id, sent_at, id);
CREATE INDEX idx_messages_received_at ON messages(received_at);

PRAGMA foreign_keys = ON;
VACUUM;
PRAGMA wal_checkpoint(TRUNCATE);

-- +goose Down
PRAGMA foreign_keys = OFF;
PRAGMA secure_delete = ON;

CREATE TABLE messages_plain (
	id TEXT PRIMARY KEY,
	room TEXT NOT NULL,
	text TEXT NOT NULL,
	sender_id TEXT NOT NULL,
	sender_username TEXT NOT NULL,
	sent_at TEXT NOT NULL,
	version INTEGER NOT NULL,
	received_at TEXT NOT NULL
);

DROP INDEX idx_messages_received_at;
DROP INDEX idx_messages_room_sent_at;
DROP TABLE messages;
DROP TABLE rooms;
ALTER TABLE messages_plain RENAME TO messages;

CREATE INDEX idx_messages_room_sent_at ON messages(room, sent_at, id);

PRAGMA foreign_keys = ON;
VACUUM;
PRAGMA wal_checkpoint(TRUNCATE);
