package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrBadRequest = errors.New("bad request")
)

type Row struct {
	Shortlink  string `json:"shortlink"`
	Longlink   string `json:"longlink"`
	Hits       int64  `json:"hits"`
	ExpiryTime int64  `json:"expiry_time"`
}

type ClickEvent struct {
	Shortlink   string
	ClickedAt   int64
	IP          string
	UserAgent   string
	Referer     string
	CountryCode string
	CityName    string
}

type Store struct {
	db         *sql.DB
	useWALMode bool
}

func Open(path string, useWALMode bool, ensureACID bool) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err = db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	if _, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS urls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			long_url TEXT NOT NULL,
			short_url TEXT NOT NULL,
			hits INTEGER NOT NULL,
			expiry_time INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create urls table: %w", err)
	}

	if _, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_short_url ON urls (short_url)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create short_url index: %w", err)
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_expiry_time ON urls (expiry_time)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create expiry_time index: %w", err)
	}

	if _, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS click_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			short_url TEXT NOT NULL,
			clicked_at INTEGER NOT NULL,
			ip TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			referer TEXT NOT NULL,
			country_code TEXT NOT NULL,
			city_name TEXT NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create click_events table: %w", err)
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_click_events_short_url ON click_events (short_url)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create click_events short_url index: %w", err)
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_click_events_clicked_at ON click_events (clicked_at)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create click_events clicked_at index: %w", err)
	}

	journalMode := "DELETE"
	synchronous := "EXTRA"
	if useWALMode {
		journalMode = "WAL"
		synchronous = "FULL"
	}
	if !ensureACID {
		if useWALMode {
			synchronous = "NORMAL"
		} else {
			synchronous = "FULL"
		}
	}

	if _, err = db.Exec(fmt.Sprintf(`PRAGMA journal_mode = %s`, journalMode)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set journal_mode: %w", err)
	}
	if _, err = db.Exec(fmt.Sprintf(`PRAGMA synchronous = %s`, synchronous)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set synchronous: %w", err)
	}
	if _, err = db.Exec(`PRAGMA temp_store = memory`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set temp_store: %w", err)
	}
	if _, err = db.Exec(`PRAGMA journal_size_limit = 8388608`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set journal_size_limit: %w", err)
	}
	if _, err = db.Exec(`PRAGMA mmap_size = 16777216`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set mmap_size: %w", err)
	}

	return &Store{db: db, useWALMode: useWALMode}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) AddLink(shortlink, longlink string, expiryDelay int64) (int64, error) {
	now := time.Now().UTC().Unix()
	expiryTime := int64(0)
	if expiryDelay > 0 {
		expiryTime = now + expiryDelay
	}

	res, err := s.db.Exec(
		`INSERT INTO urls (long_url, short_url, hits, expiry_time)
		 VALUES (?, ?, 0, ?)
		 ON CONFLICT(short_url) DO UPDATE
		 SET long_url = excluded.long_url, hits = 0, expiry_time = excluded.expiry_time
		 WHERE urls.short_url = excluded.short_url AND urls.expiry_time <= ? AND urls.expiry_time > 0`,
		longlink,
		shortlink,
		expiryTime,
		now,
	)
	if err != nil {
		return 0, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, ErrConflict
	}

	return expiryTime, nil
}

func (s *Store) FindURL(shortlink string) (string, int64, int64, error) {
	now := time.Now().UTC().Unix()
	var longURL string
	var hits int64
	var expiryTime int64

	err := s.db.QueryRow(
		`SELECT long_url, hits, expiry_time
		 FROM urls
		 WHERE short_url = ?
		 AND (expiry_time = 0 OR expiry_time > ?)`,
		shortlink,
		now,
	).Scan(&longURL, &hits, &expiryTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, 0, ErrNotFound
		}
		return "", 0, 0, err
	}

	return longURL, hits, expiryTime, nil
}

func (s *Store) FindAndAddHit(shortlink string) (string, int64, error) {
	now := time.Now().UTC().Unix()
	var longURL string
	var expiryTime int64

	err := s.db.QueryRow(
		`UPDATE urls
		 SET hits = hits + 1
		 WHERE short_url = ? AND (expiry_time = 0 OR expiry_time > ?)
		 RETURNING long_url, expiry_time`,
		shortlink,
		now,
	).Scan(&longURL, &expiryTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, ErrNotFound
		}
		return "", 0, err
	}

	return longURL, expiryTime, nil
}

func (s *Store) AddHit(shortlink string) error {
	now := time.Now().UTC().Unix()

	res, err := s.db.Exec(
		`UPDATE urls
		 SET hits = hits + 1
		 WHERE short_url = ? AND (expiry_time = 0 OR expiry_time > ?)`,
		shortlink,
		now,
	)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) EditLink(shortlink, longlink string, resetHits bool) error {
	now := time.Now().UTC().Unix()
	query := `UPDATE urls
		SET long_url = ?
		WHERE short_url = ? AND (expiry_time = 0 OR expiry_time > ?)`
	if resetHits {
		query = `UPDATE urls
			SET long_url = ?, hits = 0
			WHERE short_url = ? AND (expiry_time = 0 OR expiry_time > ?)`
	}

	res, err := s.db.Exec(query, longlink, shortlink, now)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteLink(shortlink string) error {
	res, err := s.db.Exec(`DELETE FROM urls WHERE short_url = ?`, shortlink)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetAll(pageAfter string, pageNo int64, pageSize int64) ([]Row, error) {
	now := time.Now().UTC().Unix()
	query := `
		SELECT short_url, long_url, hits, expiry_time
		FROM urls
		WHERE expiry_time = 0 OR expiry_time > ?
		ORDER BY id ASC`
	args := []any{now}

	if pageAfter != "" {
		query = `
			SELECT short_url, long_url, hits, expiry_time FROM (
				SELECT t.id, t.short_url, t.long_url, t.hits, t.expiry_time
				FROM urls AS t
				JOIN urls AS u ON u.short_url = ?
				WHERE t.id < u.id AND (t.expiry_time = 0 OR t.expiry_time > ?)
				ORDER BY t.id DESC
				LIMIT ?
			)
			ORDER BY id ASC`
		args = []any{pageAfter, now, pageSize}
	} else if pageNo > 0 {
		offset := (pageNo - 1) * pageSize
		query = `
			SELECT short_url, long_url, hits, expiry_time FROM (
				SELECT id, short_url, long_url, hits, expiry_time
				FROM urls
				WHERE expiry_time = 0 OR expiry_time > ?
				ORDER BY id DESC
				LIMIT ? OFFSET ?
			)
			ORDER BY id ASC`
		args = []any{now, pageSize, offset}
	} else if pageSize > 0 {
		query = `
			SELECT short_url, long_url, hits, expiry_time FROM (
				SELECT id, short_url, long_url, hits, expiry_time
				FROM urls
				WHERE expiry_time = 0 OR expiry_time > ?
				ORDER BY id DESC
				LIMIT ?
			)
			ORDER BY id ASC`
		args = []any{now, pageSize}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Row, 0)
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.Shortlink, &row.Longlink, &row.Hits, &row.ExpiryTime); err != nil {
			return nil, err
		}
		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (s *Store) Cleanup() error {
	now := time.Now().UTC().Unix()
	if _, err := s.db.Exec(`DELETE FROM urls WHERE ? >= expiry_time AND expiry_time > 0`, now); err != nil {
		return err
	}

	if s.useWALMode {
		if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return err
		}
	}

	if _, err := s.db.Exec(`PRAGMA optimize`); err != nil {
		return err
	}

	return nil
}

func (s *Store) RecordClickEvents(events []ClickEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO click_events
		(short_url, clicked_at, ip, user_agent, referer, country_code, city_name)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	hitIncrements := make(map[string]int64)
	for _, event := range events {
		if _, err := stmt.Exec(
			event.Shortlink,
			event.ClickedAt,
			event.IP,
			event.UserAgent,
			event.Referer,
			event.CountryCode,
			event.CityName,
		); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
		hitIncrements[event.Shortlink]++
	}

	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}

	for shortlink, inc := range hitIncrements {
		if _, err := tx.Exec(`UPDATE urls SET hits = hits + ? WHERE short_url = ?`, inc, shortlink); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}

	return nil
}
