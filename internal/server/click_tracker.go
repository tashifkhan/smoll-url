package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"log"

	"github.com/oschwald/maxminddb-golang"

	"smoll-url/internal/config"
	"smoll-url/internal/store"
)

type clickQueueItem struct {
	Shortlink string
	ClickedAt int64
	IP        string
	UserAgent string
	Referer   string
}

type clickTracker struct {
	store         *store.Store
	queue         chan clickQueueItem
	batchSize     int
	flushInterval time.Duration
	geoDB         *maxminddb.Reader

	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
	closeErr  error
}

type maxMindCityRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
}

func newClickTracker(cfg config.Config, st *store.Store) *clickTracker {
	if st == nil {
		return nil
	}

	var geoDB *maxminddb.Reader
	if cfg.MaxMindDBPath != "" {
		reader, err := maxminddb.Open(cfg.MaxMindDBPath)
		if err != nil {
			log.Printf("maxmind lookup disabled (open error): %v", err)
		} else {
			geoDB = reader
		}
	}

	t := &clickTracker{
		store:         st,
		queue:         make(chan clickQueueItem, cfg.ClickQueueSize),
		batchSize:     cfg.ClickBatchSize,
		flushInterval: time.Duration(cfg.ClickFlushIntervalMS) * time.Millisecond,
		geoDB:         geoDB,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}

	go t.run()
	return t
}

func (t *clickTracker) enqueue(item clickQueueItem) {
	if t == nil {
		return
	}

	select {
	case t.queue <- item:
	default:
		log.Printf("click queue full, dropping event for /%s", item.Shortlink)
	}
}

func (t *clickTracker) close() error {
	if t == nil {
		return nil
	}

	t.closeOnce.Do(func() {
		close(t.stopCh)
		<-t.doneCh
		if t.geoDB != nil {
			t.closeErr = t.geoDB.Close()
		}
	})

	return t.closeErr
}

func (t *clickTracker) run() {
	ticker := time.NewTicker(t.flushInterval)
	defer ticker.Stop()
	defer close(t.doneCh)

	batch := make([]clickQueueItem, 0, t.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := t.flush(batch); err != nil {
			log.Printf("click batch flush error: %v", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case item := <-t.queue:
			batch = append(batch, item)
			if len(batch) >= t.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-t.stopCh:
			for {
				select {
				case item := <-t.queue:
					batch = append(batch, item)
					if len(batch) >= t.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (t *clickTracker) flush(batch []clickQueueItem) error {
	toInsert := make([]store.ClickEvent, 0, len(batch))
	for _, item := range batch {
		countryCode, cityName := t.lookupGeo(item.IP)
		toInsert = append(toInsert, store.ClickEvent{
			Shortlink:   item.Shortlink,
			ClickedAt:   item.ClickedAt,
			IP:          item.IP,
			UserAgent:   item.UserAgent,
			Referer:     item.Referer,
			CountryCode: countryCode,
			CityName:    cityName,
		})
	}

	return t.store.RecordClickEvents(toInsert)
}

func (t *clickTracker) lookupGeo(ipAddress string) (string, string) {
	if t == nil || t.geoDB == nil || ipAddress == "" {
		return "", ""
	}

	ip := net.ParseIP(strings.TrimSpace(ipAddress))
	if ip == nil {
		return "", ""
	}

	var rec maxMindCityRecord
	if err := t.geoDB.Lookup(ip, &rec); err != nil {
		return "", ""
	}

	countryCode := strings.ToUpper(strings.TrimSpace(rec.Country.ISOCode))
	cityName := strings.TrimSpace(rec.City.Names["en"])
	return countryCode, cityName
}

func clientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	for _, header := range []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}

		for _, part := range strings.Split(value, ",") {
			ip := strings.TrimSpace(part)
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed.String()
			}
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if parsed := net.ParseIP(strings.TrimSpace(host)); parsed != nil {
			return parsed.String()
		}
	}

	ip := strings.TrimSpace(r.RemoteAddr)
	if parsed := net.ParseIP(ip); parsed != nil {
		return parsed.String()
	}

	return ""
}
