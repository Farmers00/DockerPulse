package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func InitDB(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "dockpulse.db")
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	conn.SetMaxOpenConns(1) // SQLite works best with 1 open writer in WAL mode

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'admin',
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS hosts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		driver TEXT NOT NULL,
		address TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 0,
		auth_token TEXT,
		ssh_user TEXT,
		ssh_key TEXT,
		base_dir TEXT NOT NULL DEFAULT '~/docker',
		status TEXT NOT NULL DEFAULT 'unknown',
		last_seen DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS stacks (
		id TEXT PRIMARY KEY,
		host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		path TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'unknown',
		auto_update BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(host_id, path)
	);

	CREATE TABLE IF NOT EXISTS stack_revisions (
		id TEXT PRIMARY KEY,
		stack_id TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
		revision_num INTEGER NOT NULL,
		compose_content TEXT NOT NULL,
		env_content TEXT NOT NULL,
		created_by TEXT NOT NULL,
		note TEXT,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS update_checks (
		id TEXT PRIMARY KEY,
		host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
		stack_id TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
		service_name TEXT NOT NULL,
		image_name TEXT NOT NULL,
		current_digest TEXT NOT NULL,
		remote_digest TEXT NOT NULL,
		has_update BOOLEAN NOT NULL DEFAULT 0,
		last_checked_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		username TEXT,
		action TEXT NOT NULL,
		target TEXT,
		details TEXT,
		created_at DATETIME NOT NULL
	);
	`
	_, err := db.conn.Exec(schema)
	return err
}

// User operations
func (db *DB) CountUsers() (int, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (db *DB) CreateUser(u *User) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	u.CreatedAt = time.Now().UTC()
	_, err := db.conn.Exec(
		"INSERT INTO users (id, username, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)",
		u.ID, u.Username, u.PasswordHash, u.Role, u.CreatedAt,
	)
	return err
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := db.conn.QueryRow(
		"SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Host operations
func (db *DB) ListHosts() ([]Host, error) {
	rows, err := db.conn.Query("SELECT id, name, driver, address, port, auth_token, ssh_user, base_dir, status, last_seen, created_at FROM hosts ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var h Host
		var authToken, sshUser sql.NullString
		if err := rows.Scan(&h.ID, &h.Name, &h.Driver, &h.Address, &h.Port, &authToken, &sshUser, &h.BaseDir, &h.Status, &h.LastSeen, &h.CreatedAt); err != nil {
			return nil, err
		}
		if authToken.Valid {
			h.AuthToken = authToken.String
		}
		if sshUser.Valid {
			h.SSHUser = sshUser.String
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

func (db *DB) GetHost(id string) (*Host, error) {
	h := &Host{}
	var authToken, sshUser, sshKey sql.NullString
	err := db.conn.QueryRow(
		"SELECT id, name, driver, address, port, auth_token, ssh_user, ssh_key, base_dir, status, last_seen, created_at FROM hosts WHERE id = ?",
		id,
	).Scan(&h.ID, &h.Name, &h.Driver, &h.Address, &h.Port, &authToken, &sshUser, &sshKey, &h.BaseDir, &h.Status, &h.LastSeen, &h.CreatedAt)
	if err != nil {
		return nil, err
	}
	if authToken.Valid {
		h.AuthToken = authToken.String
	}
	if sshUser.Valid {
		h.SSHUser = sshUser.String
	}
	if sshKey.Valid {
		h.SSHKey = sshKey.String
	}
	return h, nil
}

func (db *DB) CreateHost(h *Host) error {
	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	h.CreatedAt = now
	h.LastSeen = now
	_, err := db.conn.Exec(
		"INSERT INTO hosts (id, name, driver, address, port, auth_token, ssh_user, ssh_key, base_dir, status, last_seen, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		h.ID, h.Name, h.Driver, h.Address, h.Port, h.AuthToken, h.SSHUser, h.SSHKey, h.BaseDir, h.Status, h.LastSeen, h.CreatedAt,
	)
	return err
}

func (db *DB) UpdateHostStatus(id, status string) error {
	_, err := db.conn.Exec("UPDATE hosts SET status = ?, last_seen = ? WHERE id = ?", status, time.Now().UTC(), id)
	return err
}

func (db *DB) DeleteHost(id string) error {
	_, err := db.conn.Exec("DELETE FROM hosts WHERE id = ?", id)
	return err
}

// Stack operations
func (db *DB) ListStacks(hostID string) ([]Stack, error) {
	query := "SELECT id, host_id, name, path, status, auto_update, created_at, updated_at FROM stacks"
	var rows *sql.Rows
	var err error
	if hostID != "" {
		query += " WHERE host_id = ? ORDER BY name ASC"
		rows, err = db.conn.Query(query, hostID)
	} else {
		query += " ORDER BY name ASC"
		rows, err = db.conn.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stacks := make([]Stack, 0)
	for rows.Next() {
		var s Stack
		if err := rows.Scan(&s.ID, &s.HostID, &s.Name, &s.Path, &s.Status, &s.AutoUpdate, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		stacks = append(stacks, s)
	}
	return stacks, nil
}

func (db *DB) GetStack(id string) (*Stack, error) {
	s := &Stack{}
	err := db.conn.QueryRow(
		"SELECT id, host_id, name, path, status, auto_update, created_at, updated_at FROM stacks WHERE id = ?",
		id,
	).Scan(&s.ID, &s.HostID, &s.Name, &s.Path, &s.Status, &s.AutoUpdate, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (db *DB) UpsertStack(s *Stack) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	_, err := db.conn.Exec(`
		INSERT INTO stacks (id, host_id, name, path, status, auto_update, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(host_id, path) DO UPDATE SET
			name = excluded.name,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, s.ID, s.HostID, s.Name, s.Path, s.Status, s.AutoUpdate, s.CreatedAt, s.UpdatedAt)
	return err
}

func (db *DB) DeleteStack(id string) error {
	_, err := db.conn.Exec("DELETE FROM stacks WHERE id = ?", id)
	return err
}

// Revision operations
func (db *DB) CreateRevision(r *StackRevision) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	r.CreatedAt = time.Now().UTC()

	var nextRev int
	err := db.conn.QueryRow("SELECT COALESCE(MAX(revision_num), 0) + 1 FROM stack_revisions WHERE stack_id = ?", r.StackID).Scan(&nextRev)
	if err != nil {
		nextRev = 1
	}
	r.RevisionNum = nextRev

	_, err = db.conn.Exec(`
		INSERT INTO stack_revisions (id, stack_id, revision_num, compose_content, env_content, created_by, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.StackID, r.RevisionNum, r.ComposeContent, r.EnvContent, r.CreatedBy, r.Note, r.CreatedAt)
	return err
}

func (db *DB) ListRevisions(stackID string) ([]StackRevision, error) {
	rows, err := db.conn.Query(`
		SELECT id, stack_id, revision_num, compose_content, env_content, created_by, note, created_at
		FROM stack_revisions WHERE stack_id = ? ORDER BY revision_num DESC
	`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	revs := make([]StackRevision, 0)
	for rows.Next() {
		var r StackRevision
		var note sql.NullString
		if err := rows.Scan(&r.ID, &r.StackID, &r.RevisionNum, &r.ComposeContent, &r.EnvContent, &r.CreatedBy, &note, &r.CreatedAt); err != nil {
			return nil, err
		}
		if note.Valid {
			r.Note = note.String
		}
		revs = append(revs, r)
	}
	return revs, nil
}
