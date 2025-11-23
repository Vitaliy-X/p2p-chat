package sqlite

type migration struct {
	version    int
	statements []string
}

var migrations = []migration{
	{
		version: 1,
		statements: []string{
			`CREATE TABLE IF NOT EXISTS messages (
				id TEXT PRIMARY KEY,
				room TEXT NOT NULL,
				text TEXT NOT NULL,
				sender_id TEXT NOT NULL,
				sender_username TEXT NOT NULL,
				sent_at TEXT NOT NULL,
				version INTEGER NOT NULL,
				received_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_messages_room_sent_at ON messages(room, sent_at, id)`,
			`CREATE TABLE IF NOT EXISTS settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		},
	},
}
