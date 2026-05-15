package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Krembovan/techat/internal/model"

	_ "modernc.org/sqlite"
)

type Database struct {
	db   *sql.DB
	path string
}

func New(path string) (*Database, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragmas: %w", err)
		}
	}

	d := &Database{db: db, path: path}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Database) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sender TEXT NOT NULL,
		recipient TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL,
		nonce TEXT NOT NULL DEFAULT '',
		is_encrypted INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		file_path TEXT DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS contacts (
		username TEXT PRIMARY KEY,
		pubkey TEXT NOT NULL DEFAULT '',
		last_seen TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_messages_recipient ON messages(recipient);
	CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender);
	`
	_, err := d.db.Exec(schema)
	return err
}

func (d *Database) SaveMessage(msg model.StoredMessage) (int64, error) {
	if msg.Timestamp == "" {
		msg.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	enc := 0
	if msg.IsEncrypted {
		enc = 1
	}
	result, err := d.db.Exec(
		`INSERT INTO messages (sender, recipient, content, nonce, is_encrypted, created_at, file_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.Sender, msg.Recipient, msg.Content, msg.Nonce, enc, msg.Timestamp, msg.FilePath,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *Database) GetMessages(contact string, limit, offset int) ([]model.StoredMessage, error) {
	query := `SELECT id, sender, recipient, content, nonce, is_encrypted, created_at, file_path
			  FROM messages
			  WHERE (recipient = ? OR sender = ?)
			  ORDER BY created_at DESC
			  LIMIT ? OFFSET ?`

	rows, err := d.db.Query(query, contact, contact, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.StoredMessage
	for rows.Next() {
		var msg model.StoredMessage
		var isEnc int
		if err := rows.Scan(&msg.ID, &msg.Sender, &msg.Recipient, &msg.Content,
			&msg.Nonce, &isEnc, &msg.Timestamp, &msg.FilePath); err != nil {
			return nil, err
		}
		msg.IsEncrypted = isEnc == 1
		messages = append(messages, msg)
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (d *Database) GetGlobalMessages(limit, offset int) ([]model.StoredMessage, error) {
	query := `SELECT id, sender, recipient, content, nonce, is_encrypted, created_at, file_path
			  FROM messages
			  WHERE recipient = 'global'
			  ORDER BY created_at DESC
			  LIMIT ? OFFSET ?`

	rows, err := d.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.StoredMessage
	for rows.Next() {
		var msg model.StoredMessage
		var isEnc int
		if err := rows.Scan(&msg.ID, &msg.Sender, &msg.Recipient, &msg.Content,
			&msg.Nonce, &isEnc, &msg.Timestamp, &msg.FilePath); err != nil {
			return nil, err
		}
		msg.IsEncrypted = isEnc == 1
		messages = append(messages, msg)
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (d *Database) SaveContact(username, pubKey string) error {
	_, err := d.db.Exec(
		`INSERT INTO contacts (username, pubkey, last_seen) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(username) DO UPDATE SET pubkey = COALESCE(NULLIF(?, ''), pubkey), last_seen = datetime('now')`,
		username, pubKey, pubKey,
	)
	return err
}

func (d *Database) UpdateContactLastSeen(username string) error {
	_, err := d.db.Exec(
		`UPDATE contacts SET last_seen = datetime('now') WHERE username = ?`,
		username,
	)
	return err
}

func (d *Database) GetContacts() ([]model.User, error) {
	rows, err := d.db.Query(`SELECT username, pubkey FROM contacts ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.Username, &u.PubKey); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *Database) GetContact(username string) (*model.User, error) {
	row := d.db.QueryRow(`SELECT username, pubkey FROM contacts WHERE username = ?`, username)
	var u model.User
	if err := row.Scan(&u.Username, &u.PubKey); err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *Database) GetPubKey(username string) (string, error) {
	var pubKey string
	err := d.db.QueryRow(`SELECT pubkey FROM contacts WHERE username = ?`, username).Scan(&pubKey)
	return pubKey, err
}

func (d *Database) HasContact(username string) bool {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM contacts WHERE username = ?`, username).Scan(&count)
	return count > 0
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) Wipe() error {
	d.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	d.db.Close()

	for i := 0; i < 3; i++ {
		if err := overwriteFile(d.path); err != nil {
			return err
		}
	}
	os.Remove(d.path)
	os.Remove(d.path + "-wal")
	os.Remove(d.path + "-shm")
	return nil
}

func overwriteFile(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	size := info.Size()
	if size == 0 {
		return nil
	}

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 4096)
	var written int64
	for written < size {
		n := copy(buf, make([]byte, 4096))
		if remaining := size - written; int64(n) > remaining {
			n = int(remaining)
		}
		if _, err := f.Write(buf[:n]); err != nil {
			return err
		}
		written += int64(n)
	}
	return f.Sync()
}

func (d *Database) GetPrivateChatContacts(username string) ([]string, error) {
	rows, err := d.db.Query(
		`SELECT DISTINCT
			CASE WHEN sender = ? THEN recipient ELSE sender END
		 FROM messages
		 WHERE (sender = ? OR recipient = ?)
		 AND recipient != 'global'
		 AND sender != 'global'
		 ORDER BY created_at DESC`,
		username, username, username,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []string
	seen := make(map[string]bool)
	for rows.Next() {
		var contact string
		if err := rows.Scan(&contact); err != nil {
			return nil, err
		}
		if contact != "" && !seen[contact] {
			contacts = append(contacts, contact)
			seen[contact] = true
		}
	}
	return contacts, nil
}

func (d *Database) GetOrCreateConversation(user1, user2 string) error {
	_, err := d.db.Exec(
		`INSERT OR IGNORE INTO messages (sender, recipient, content, nonce, is_encrypted, created_at, file_path)
		 VALUES (?, ?, '', '', 0, datetime('now'), '')`,
		user1, user2,
	)
	return err
}

func (d *Database) SearchMessages(query string, limit int) ([]model.StoredMessage, error) {
	q := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"
	rows, err := d.db.Query(
		`SELECT id, sender, recipient, content, nonce, is_encrypted, created_at, file_path
		 FROM messages
		 WHERE content LIKE ? ESCAPE '\\'
		 ORDER BY created_at DESC LIMIT ?`,
		q, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.StoredMessage
	for rows.Next() {
		var msg model.StoredMessage
		var isEnc int
		if err := rows.Scan(&msg.ID, &msg.Sender, &msg.Recipient, &msg.Content,
			&msg.Nonce, &isEnc, &msg.Timestamp, &msg.FilePath); err != nil {
			return nil, err
		}
		msg.IsEncrypted = isEnc == 1
		messages = append(messages, msg)
	}
	return messages, nil
}
