package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Commitment status constants
const (
	StatusUnverified = "unverified"
	StatusBacked     = "backed"
	StatusAlerted    = "alerted"
	StatusResolved   = "resolved"
	StatusExpired    = "expired"
)

// Commitment category constants
const (
	CategoryTemporal    = "temporal"
	CategoryScheduled   = "scheduled"
	CategoryFollowup    = "followup"
	CategoryConditional = "conditional"
)

// ErrNotFound is returned when a commitment is not found
var ErrNotFound = errors.New("commitment not found")

// Commitment represents a tracked commitment in the database
type Commitment struct {
	ID          string     `json:"id"`
	DetectedAt  time.Time  `json:"detected_at"`
	Source      string     `json:"source"`
	MessageID   string     `json:"message_id"`
	Text        string     `json:"text"`
	Category    string     `json:"category"`
	BackedBy    []string   `json:"backed_by"`
	Status      string     `json:"status"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastChecked *time.Time `json:"last_checked,omitempty"`
	AlertCount  int        `json:"alert_count"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ListFilter specifies filters for listing commitments
type ListFilter struct {
	Status   string
	Category string
	Since    *time.Duration
}

// Store wraps a SQLite database for commitment storage
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS commitments (
	id            TEXT PRIMARY KEY,
	detected_at   INTEGER NOT NULL,
	source        TEXT NOT NULL,
	message_id    TEXT NOT NULL,
	text          TEXT NOT NULL,
	category      TEXT NOT NULL,
	backed_by     TEXT NOT NULL DEFAULT '[]',
	status        TEXT NOT NULL DEFAULT 'unverified',
	expires_at    INTEGER,
	last_checked  INTEGER,
	alert_count   INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_status ON commitments(status);
CREATE INDEX IF NOT EXISTS idx_source ON commitments(source);
CREATE INDEX IF NOT EXISTS idx_expires_at ON commitments(expires_at);
CREATE INDEX IF NOT EXISTS idx_detected_at ON commitments(detected_at);
`

// Open creates or opens a SQLite database at the given path
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// Insert adds a new commitment to the database
func (s *Store) Insert(c Commitment) error {
	backedByJSON, err := json.Marshal(c.BackedBy)
	if err != nil {
		return fmt.Errorf("marshal backed_by: %w", err)
	}

	var expiresAt *int64
	if c.ExpiresAt != nil {
		v := c.ExpiresAt.Unix()
		expiresAt = &v
	}

	_, err = s.db.Exec(
		`INSERT INTO commitments (id, detected_at, source, message_id, text, category, backed_by, status, expires_at, alert_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.DetectedAt.Unix(), c.Source, c.MessageID, c.Text, c.Category,
		string(backedByJSON), c.Status, expiresAt, c.AlertCount,
		c.CreatedAt.Unix(), c.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert commitment: %w", err)
	}
	return nil
}

// Get retrieves a single commitment by ID
func (s *Store) Get(id string) (Commitment, error) {
	row := s.db.QueryRow(
		`SELECT id, detected_at, source, message_id, text, category, backed_by, status, expires_at, last_checked, alert_count, created_at, updated_at
		 FROM commitments WHERE id = ?`, id,
	)
	return scanCommitment(row)
}

// List returns commitments matching the given filters, ordered by detected_at descending
func (s *Store) List(f ListFilter) ([]Commitment, error) {
	query := `SELECT id, detected_at, source, message_id, text, category, backed_by, status, expires_at, last_checked, alert_count, created_at, updated_at FROM commitments WHERE 1=1`
	args := []any{}

	if f.Status != "" {
		query += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Category != "" {
		query += " AND category = ?"
		args = append(args, f.Category)
	}
	if f.Since != nil {
		cutoff := time.Now().Add(-*f.Since).Unix()
		query += " AND detected_at >= ?"
		args = append(args, cutoff)
	}

	query += " ORDER BY detected_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list commitments: %w", err)
	}
	defer rows.Close()

	var result []Commitment
	for rows.Next() {
		c, err := scanCommitmentFromRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// UpdateStatus updates the status, backed_by, and last_checked of a commitment
func (s *Store) UpdateStatus(id string, status string, mechanisms []string, lastChecked time.Time) error {
	if mechanisms == nil {
		mechanisms = []string{}
	}
	backedByJSON, err := json.Marshal(mechanisms)
	if err != nil {
		return fmt.Errorf("marshal backed_by: %w", err)
	}

	res, err := s.db.Exec(
		`UPDATE commitments SET status = ?, backed_by = ?, last_checked = ?, updated_at = ? WHERE id = ?`,
		status, string(backedByJSON), lastChecked.Unix(), time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementAlertCount increments the alert_count for a commitment
func (s *Store) IncrementAlertCount(id string) error {
	res, err := s.db.Exec(
		`UPDATE commitments SET alert_count = alert_count + 1, updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("increment alert count: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ExpireStale transitions commitments whose expires_at has passed to "expired" status.
// Only affects commitments with status "unverified" or "alerted" (not backed/resolved/expired).
// Returns the number of commitments expired.
func (s *Store) ExpireStale(now time.Time) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE commitments SET status = ?, updated_at = ?
		 WHERE expires_at IS NOT NULL AND expires_at < ? AND status IN (?, ?)`,
		StatusExpired, now.Unix(), now.Unix(), StatusUnverified, StatusAlerted,
	)
	if err != nil {
		return 0, fmt.Errorf("expire stale: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

func scanRow(sc scanner) (Commitment, error) {
	var c Commitment
	var detectedAt, createdAt, updatedAt int64
	var expiresAt, lastChecked *int64
	var backedByJSON string

	err := sc.Scan(&c.ID, &detectedAt, &c.Source, &c.MessageID, &c.Text, &c.Category,
		&backedByJSON, &c.Status, &expiresAt, &lastChecked, &c.AlertCount, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Commitment{}, ErrNotFound
		}
		return Commitment{}, fmt.Errorf("scan commitment: %w", err)
	}

	c.DetectedAt = time.Unix(detectedAt, 0)
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)

	if expiresAt != nil {
		t := time.Unix(*expiresAt, 0)
		c.ExpiresAt = &t
	}
	if lastChecked != nil {
		t := time.Unix(*lastChecked, 0)
		c.LastChecked = &t
	}

	if err := json.Unmarshal([]byte(backedByJSON), &c.BackedBy); err != nil {
		return Commitment{}, fmt.Errorf("unmarshal backed_by: %w", err)
	}

	return c, nil
}

func scanCommitment(row *sql.Row) (Commitment, error) {
	return scanRow(row)
}

func scanCommitmentFromRows(rows *sql.Rows) (Commitment, error) {
	return scanRow(rows)
}
