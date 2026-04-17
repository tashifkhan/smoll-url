package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
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

func (s *Store) EditLink(originalShortlink, shortlink, longlink string, resetHits bool) error {
	now := time.Now().UTC().Unix()
	query := `UPDATE urls
		SET short_url = ?, long_url = ?
		WHERE short_url = ? AND (expiry_time = 0 OR expiry_time > ?)`
	if resetHits {
		query = `UPDATE urls
			SET short_url = ?, long_url = ?, hits = 0
			WHERE short_url = ? AND (expiry_time = 0 OR expiry_time > ?)`
	}

	res, err := s.db.Exec(query, shortlink, longlink, originalShortlink, now)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrConflict
		}
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

func isUniqueConstraintErr(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
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

// ---- Analytics Types ----

type CountStat struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type TimelineStat struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type ClickAnalytics struct {
	Slug        string         `json:"slug"`
	TotalClicks int64          `json:"total_clicks"`
	Countries   []CountStat    `json:"countries"`
	Devices     []CountStat    `json:"devices"`
	Browsers    []CountStat    `json:"browsers"`
	Referrers   []CountStat    `json:"referrers"`
	Timeline    []TimelineStat `json:"timeline"`
}

func (s *Store) GetClickAnalytics(shortlink string, days int) (*ClickAnalytics, error) {
	var since int64
	if days > 0 {
		since = time.Now().UTC().AddDate(0, 0, -days).Unix()
	}

	result := &ClickAnalytics{
		Slug:      shortlink,
		Countries: []CountStat{},
		Devices:   []CountStat{},
		Browsers:  []CountStat{},
		Referrers: []CountStat{},
		Timeline:  []TimelineStat{},
	}

	// Total clicks
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM click_events WHERE short_url = ? AND clicked_at >= ?`,
		shortlink, since,
	).Scan(&result.TotalClicks); err != nil {
		return nil, err
	}

	// Top countries
	{
		rows, err := s.db.Query(
			`SELECT COALESCE(NULLIF(country_code,''),'Unknown') as cc, COUNT(*) as cnt
			 FROM click_events WHERE short_url = ? AND clicked_at >= ?
			 GROUP BY cc ORDER BY cnt DESC LIMIT 10`,
			shortlink, since,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var stat CountStat
			if err := rows.Scan(&stat.Label, &stat.Count); err != nil {
				rows.Close()
				return nil, err
			}
			result.Countries = append(result.Countries, stat)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	// Referrers (domain-extracted)
	{
		rows, err := s.db.Query(
			`SELECT COALESCE(NULLIF(referer,''),'direct') as ref, COUNT(*) as cnt
			 FROM click_events WHERE short_url = ? AND clicked_at >= ?
			 GROUP BY referer ORDER BY cnt DESC LIMIT 20`,
			shortlink, since,
		)
		if err != nil {
			return nil, err
		}
		refMap := make(map[string]int64)
		for rows.Next() {
			var ref string
			var cnt int64
			if err := rows.Scan(&ref, &cnt); err != nil {
				rows.Close()
				return nil, err
			}
			refMap[extractRefDomain(ref)] += cnt
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for label, count := range refMap {
			result.Referrers = append(result.Referrers, CountStat{Label: label, Count: count})
		}
		sort.Slice(result.Referrers, func(i, j int) bool {
			return result.Referrers[i].Count > result.Referrers[j].Count
		})
		if len(result.Referrers) > 10 {
			result.Referrers = result.Referrers[:10]
		}
	}

	// Device + browser classification from user agents
	{
		rows, err := s.db.Query(
			`SELECT user_agent FROM click_events WHERE short_url = ? AND clicked_at >= ?`,
			shortlink, since,
		)
		if err != nil {
			return nil, err
		}
		deviceCounts := make(map[string]int64)
		browserCounts := make(map[string]int64)
		for rows.Next() {
			var ua string
			if err := rows.Scan(&ua); err != nil {
				rows.Close()
				return nil, err
			}
			deviceCounts[classifyDevice(ua)]++
			browserCounts[classifyBrowser(ua)]++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for label, count := range deviceCounts {
			result.Devices = append(result.Devices, CountStat{Label: label, Count: count})
		}
		sort.Slice(result.Devices, func(i, j int) bool {
			return result.Devices[i].Count > result.Devices[j].Count
		})
		for label, count := range browserCounts {
			result.Browsers = append(result.Browsers, CountStat{Label: label, Count: count})
		}
		sort.Slice(result.Browsers, func(i, j int) bool {
			return result.Browsers[i].Count > result.Browsers[j].Count
		})
	}

	// Click timeline — fill all days in range (including zero-click days)
	{
		nowUTC := time.Now().UTC()
		sinceTime := time.Unix(since, 0).UTC()
		if days > 0 {
			sinceTime = nowUTC.AddDate(0, 0, -days)
		}
		sinceDay := time.Date(sinceTime.Year(), sinceTime.Month(), sinceTime.Day(), 0, 0, 0, 0, time.UTC)
		today := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)

		rows, err := s.db.Query(
			`SELECT strftime('%Y-%m-%d', datetime(clicked_at, 'unixepoch')) as day, COUNT(*) as cnt
			 FROM click_events WHERE short_url = ? AND clicked_at >= ?
			 GROUP BY day ORDER BY day ASC`,
			shortlink, since,
		)
		if err != nil {
			return nil, err
		}
		dayMap := make(map[string]int64)
		var firstDay string
		for rows.Next() {
			var stat TimelineStat
			if err := rows.Scan(&stat.Date, &stat.Count); err != nil {
				rows.Close()
				return nil, err
			}
			dayMap[stat.Date] = stat.Count
			if firstDay == "" {
				firstDay = stat.Date
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		var start time.Time
		if days > 0 {
			start = sinceDay
		} else if firstDay != "" {
			start, _ = time.Parse("2006-01-02", firstDay)
		} else {
			start = today
		}

		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
			key := d.Format("2006-01-02")
			result.Timeline = append(result.Timeline, TimelineStat{Date: key, Count: dayMap[key]})
		}
	}

	return result, nil
}

func extractRefDomain(ref string) string {
	if ref == "direct" || ref == "" {
		return "Direct"
	}
	if idx := strings.Index(ref, "://"); idx >= 0 {
		rest := ref[idx+3:]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			rest = rest[:i]
		}
		if i := strings.IndexByte(rest, '?'); i >= 0 {
			rest = rest[:i]
		}
		return rest
	}
	if len(ref) > 50 {
		return ref[:50] + "..."
	}
	return ref
}

func classifyDevice(ua string) string {
	if ua == "" {
		return "Unknown"
	}
	uaL := strings.ToLower(ua)
	if strings.Contains(uaL, "ipad") || (strings.Contains(uaL, "android") && !strings.Contains(uaL, "mobile")) {
		return "Tablet"
	}
	if strings.Contains(uaL, "mobile") || strings.Contains(uaL, "iphone") || strings.Contains(uaL, "ipod") {
		return "Mobile"
	}
	return "Desktop"
}

func classifyBrowser(ua string) string {
	if ua == "" {
		return "Unknown"
	}
	uaL := strings.ToLower(ua)
	switch {
	case strings.Contains(uaL, "edg/") || strings.Contains(uaL, "edge/"):
		return "Edge"
	case strings.Contains(uaL, "opr/") || strings.Contains(uaL, "opera"):
		return "Opera"
	case strings.Contains(uaL, "chrome") || strings.Contains(uaL, "crios") || strings.Contains(uaL, "chromium"):
		return "Chrome"
	case strings.Contains(uaL, "firefox") || strings.Contains(uaL, "fxios"):
		return "Firefox"
	case strings.Contains(uaL, "safari"):
		return "Safari"
	case strings.Contains(uaL, "curl"):
		return "cURL"
	case strings.Contains(uaL, "wget"):
		return "Wget"
	case strings.Contains(uaL, "python"):
		return "Python"
	case strings.Contains(uaL, "go-http"):
		return "Go HTTP"
	default:
		return "Other"
	}
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
