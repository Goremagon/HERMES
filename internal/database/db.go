package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const startupTimeout = 5 * time.Second

type Message struct {
	ID        int64     `json:"id"`
	ChannelID int64     `json:"channel_id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func InitDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("database path is required")
	}

	dir := filepath.Dir(dbPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable wal mode: %w", err)
	}

	if err := createSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func CreateMessage(ctx context.Context, db *sql.DB, userID, channelID int64, content string) (Message, error) {
	result, err := db.ExecContext(ctx, `INSERT INTO messages (channel_id, user_id, content) VALUES (?, ?, ?)`, channelID, userID, content)
	if err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
	}

	messageID, err := result.LastInsertId()
	if err != nil {
		return Message{}, fmt.Errorf("get message id: %w", err)
	}

	message, err := getMessageByID(ctx, db, messageID)
	if err != nil {
		return Message{}, err
	}

	return message, nil
}

func GetMessages(ctx context.Context, db *sql.DB, channelID int64, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.QueryContext(ctx, `
SELECT m.id, m.channel_id, m.user_id, u.username, COALESCE(u.avatar_url, ''), m.content, m.created_at
FROM messages m
JOIN users u ON u.id = m.user_id
WHERE m.channel_id = ?
ORDER BY m.id DESC
LIMIT ?`, channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	messages := make([]Message, 0, limit)
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.UserID, &msg.Username, &msg.AvatarURL, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func getMessageByID(ctx context.Context, db *sql.DB, id int64) (Message, error) {
	var msg Message
	err := db.QueryRowContext(ctx, `
SELECT m.id, m.channel_id, m.user_id, u.username, COALESCE(u.avatar_url, ''), m.content, m.created_at
FROM messages m
JOIN users u ON u.id = m.user_id
WHERE m.id = ?`, id).Scan(&msg.ID, &msg.ChannelID, &msg.UserID, &msg.Username, &msg.AvatarURL, &msg.Content, &msg.CreatedAt)
	if err != nil {
		return Message{}, fmt.Errorf("fetch message: %w", err)
	}
	return msg, nil
}

func createSchema(ctx context.Context, db *sql.DB) error {
	const schemaSQL = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	avatar_url TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	expires_at DATETIME NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS channels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	content TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
`

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	if err := ensureUserAvatarColumn(ctx, db); err != nil {
		return err
	}

	return nil
}

func ensureUserAvatarColumn(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(users)`) 
	if err != nil {
		return fmt.Errorf("inspect users table: %w", err)
	}
	defer rows.Close()

	hasAvatar := false
	for rows.Next() {
		var (
			cid       int
			name      string
			typeName  string
			notNull   int
			defaultV  sql.NullString
			primaryID int
		)
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultV, &primaryID); err != nil {
			return fmt.Errorf("scan users pragma: %w", err)
		}
		if strings.EqualFold(name, "avatar_url") {
			hasAvatar = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate users pragma: %w", err)
	}

	if hasAvatar {
		return nil
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN avatar_url TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add avatar_url column: %w", err)
	}

	return nil
}
