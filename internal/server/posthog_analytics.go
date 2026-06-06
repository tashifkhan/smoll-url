package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"smoll-url/internal/auth"
)

type postHogStatsResponse struct {
	Metadata   postHogStatsMetadata   `json:"metadata"`
	Timeseries []postHogTimeseriesRow `json:"timeseries"`
	Stats      postHogStatsBreakdowns `json:"stats"`
}

type postHogStatsMetadata struct {
	ExportDate string `json:"export_date"`
	Source     string `json:"source"`
}

type postHogTimeseriesRow struct {
	Date       string  `json:"date"`
	Pageviews  int64   `json:"pageviews"`
	Visitors   int64   `json:"visitors"`
	BounceRate float64 `json:"bounce_rate"`
}

type postHogStatEntry struct {
	Key       string `json:"key"`
	Pageviews int64  `json:"pageviews"`
	Visitors  int64  `json:"visitors"`
}

type postHogStatsBreakdowns struct {
	Path       []postHogStatEntry `json:"path"`
	DeviceType []postHogStatEntry `json:"device_type"`
	Referrer   []postHogStatEntry `json:"referrer"`
	OSName     []postHogStatEntry `json:"os_name"`
	Country    []postHogStatEntry `json:"country"`
}

type postHogQueryResponse struct {
	Results [][]any `json:"results"`
}

func (s *Server) handlePostHogAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	if !apiResult.Success && !s.isSessionValid(r) {
		writeJSON(w, http.StatusUnauthorized, apiResult)
		return
	}

	if s.cfg.PostHogProjectID == "" || s.cfg.PostHogPersonalAPIKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, JSONResponse{Success: false, Error: true, Reason: "PostHog analytics API is not configured"})
		return
	}

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n >= 0 && n <= 3650 {
			days = n
		}
	}
	queryDays := days
	if queryDays == 0 {
		queryDays = 3650
	}

	timeseries, err := s.fetchPostHogTimeseries(queryDays)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, JSONResponse{Success: false, Error: true, Reason: err.Error()})
		return
	}

	breakdowns := postHogStatsBreakdowns{}
	breakdowns.Path, err = s.fetchPostHogBreakdown("properties.$pathname", queryDays, 12)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, JSONResponse{Success: false, Error: true, Reason: err.Error()})
		return
	}
	breakdowns.Referrer, _ = s.fetchPostHogBreakdown("properties.$referring_domain", queryDays, 12)
	breakdowns.DeviceType, _ = s.fetchPostHogBreakdown("properties.$device_type", queryDays, 8)
	breakdowns.OSName, _ = s.fetchPostHogBreakdown("properties.$os", queryDays, 8)
	breakdowns.Country, _ = s.fetchPostHogBreakdown("properties.$geoip_country_code", queryDays, 12)

	writeJSON(w, http.StatusOK, postHogStatsResponse{
		Metadata: postHogStatsMetadata{
			ExportDate: time.Now().UTC().Format(time.RFC3339),
			Source:     "posthog",
		},
		Timeseries: timeseries,
		Stats:      breakdowns,
	})
}

func (s *Server) fetchPostHogTimeseries(days int) ([]postHogTimeseriesRow, error) {
	query := fmt.Sprintf(`
SELECT
    toDate(timestamp) as d,
    count() as pageviews,
    count(DISTINCT distinct_id) as visitors
FROM events
WHERE event = '$pageview'
    AND timestamp > now() - INTERVAL %d DAY
GROUP BY d
ORDER BY d ASC
`, days)

	rows, err := s.queryPostHog(query)
	if err != nil {
		return nil, err
	}

	out := make([]postHogTimeseriesRow, 0, len(rows))
	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		out = append(out, postHogTimeseriesRow{
			Date:      stringValue(row[0]),
			Pageviews: int64Value(row[1]),
			Visitors:  int64Value(row[2]),
		})
	}
	return out, nil
}

func (s *Server) fetchPostHogBreakdown(field string, days, limit int) ([]postHogStatEntry, error) {
	query := fmt.Sprintf(`
SELECT
    %s as key,
    count() as pageviews,
    count(DISTINCT distinct_id) as visitors
FROM events
WHERE event = '$pageview'
    AND timestamp > now() - INTERVAL %d DAY
    AND %s IS NOT NULL
    AND %s != ''
GROUP BY key
ORDER BY pageviews DESC
LIMIT %d
`, field, days, field, field, limit)

	rows, err := s.queryPostHog(query)
	if err != nil {
		return nil, err
	}

	out := make([]postHogStatEntry, 0, len(rows))
	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		out = append(out, postHogStatEntry{
			Key:       stringValue(row[0]),
			Pageviews: int64Value(row[1]),
			Visitors:  int64Value(row[2]),
		})
	}
	return out, nil
}

func (s *Server) queryPostHog(hogQL string) ([][]any, error) {
	body, err := json.Marshal(map[string]any{
		"query": map[string]string{
			"kind":  "HogQLQuery",
			"query": hogQL,
		},
	})
	if err != nil {
		return nil, err
	}

	url := s.cfg.PostHogBaseURL + "/api/projects/" + s.cfg.PostHogProjectID + "/query/"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.PostHogPersonalAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("PostHog API returned %s", res.Status)
	}

	var decoded postHogQueryResponse
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded.Results, nil
}

func stringValue(v any) string {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func int64Value(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		v, _ := n.Int64()
		return v
	default:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(v)), 10, 64)
		return parsed
	}
}
