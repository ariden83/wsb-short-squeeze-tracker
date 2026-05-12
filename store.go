package main

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	return s, s.migrate()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS posts (
			id          TEXT PRIMARY KEY,
			title       TEXT NOT NULL,
			body        TEXT NOT NULL DEFAULT '',
			seen_at     INTEGER NOT NULL,
			translation TEXT NOT NULL DEFAULT '',
			sentiment   TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS mentions (
			ticker  TEXT NOT NULL,
			post_id TEXT NOT NULL,
			seen_at INTEGER NOT NULL,
			PRIMARY KEY (ticker, post_id)
		);
		CREATE TABLE IF NOT EXISTS cursor (
			id         INTEGER PRIMARY KEY CHECK (id = 1),
			last_post  TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0
		);
		INSERT OR IGNORE INTO cursor (id, last_post, updated_at) VALUES (1, '', 0);
	`)
	if err != nil {
		return err
	}
	// Add columns to existing databases (ignore error if already present)
	s.db.Exec(`ALTER TABLE posts ADD COLUMN translation TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE posts ADD COLUMN sentiment TEXT NOT NULL DEFAULT ''`)
	return nil
}

func (s *Store) GetCursor() (string, error) {
	var last string
	err := s.db.QueryRow(`SELECT last_post FROM cursor WHERE id = 1`).Scan(&last)
	return last, err
}

func (s *Store) SetCursor(postID string) error {
	_, err := s.db.Exec(
		`UPDATE cursor SET last_post = ?, updated_at = ? WHERE id = 1`,
		postID, time.Now().Unix(),
	)
	return err
}

func (s *Store) Reset() error {
	_, err := s.db.Exec(`DELETE FROM mentions; UPDATE cursor SET last_post = '', updated_at = 0 WHERE id = 1`)
	return err
}

func (s *Store) SavePost(id, title, body string, seenAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO posts (id, title, body, seen_at) VALUES (?, ?, ?, ?)`,
		id, title, body, seenAt.Unix(),
	)
	return err
}

type PostSummary struct {
	ID          string
	Title       string
	SeenAt      time.Time
	SqueezeHit  bool
	Translation string
	Sentiment   string
}

// GetPostsForTicker returns up to limit posts mentioning ticker, sorted by
// squeeze relevance first (title or body contains "squeeze"), then recency.
func (s *Store) GetPostsForTicker(ticker string, limit int) ([]PostSummary, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.title, p.seen_at,
		       (p.title LIKE '%squeeze%' OR p.body LIKE '%squeeze%') AS squeeze_hit,
		       p.translation, p.sentiment
		FROM posts p
		JOIN mentions m ON m.post_id = p.id
		WHERE m.ticker = ?
		ORDER BY squeeze_hit DESC, p.seen_at DESC
		LIMIT ?
	`, ticker, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []PostSummary
	for rows.Next() {
		var ps PostSummary
		var unix int64
		var hit int
		if err := rows.Scan(&ps.ID, &ps.Title, &unix, &hit, &ps.Translation, &ps.Sentiment); err != nil {
			return nil, err
		}
		ps.SeenAt = time.Unix(unix, 0)
		ps.SqueezeHit = hit != 0
		posts = append(posts, ps)
	}
	return posts, rows.Err()
}

func (s *Store) UpdatePostAnalysis(id, translation, sentiment string) error {
	_, err := s.db.Exec(
		`UPDATE posts SET translation = ?, sentiment = ? WHERE id = ?`,
		translation, sentiment, id,
	)
	return err
}

func (s *Store) SaveMentions(tickers []string, postID string, seenAt time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO mentions (ticker, post_id, seen_at) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, t := range tickers {
		if _, err := stmt.Exec(t, postID, seenAt.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type TickerStat struct {
	Ticker   string
	Count    int
	LastSeen time.Time
}

// GetStats returns tickers mentioned in the last `since` duration, sorted by last seen desc.
func (s *Store) GetStats(since time.Duration) ([]TickerStat, error) {
	cutoff := time.Now().Add(-since).Unix()
	rows, err := s.db.Query(`
		SELECT ticker, COUNT(*) as cnt, MAX(seen_at) as last_seen
		FROM mentions
		WHERE seen_at >= ?
		GROUP BY ticker
		ORDER BY cnt DESC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []TickerStat
	for rows.Next() {
		var ts TickerStat
		var lastUnix int64
		if err := rows.Scan(&ts.Ticker, &ts.Count, &lastUnix); err != nil {
			return nil, err
		}
		ts.LastSeen = time.Unix(lastUnix, 0)
		stats = append(stats, ts)
	}
	return stats, rows.Err()
}
