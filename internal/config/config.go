package config

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress          string
	Port                   int
	DBPath                 string
	RedisURL               string
	RedisCacheKeyPrefix    string
	RedisCacheTimeoutMS    int
	ClickQueueSize         int
	ClickBatchSize         int
	ClickFlushIntervalMS   int
	MaxMindDBPath          string
	CacheControlHeader     string
	DisableFrontend        bool
	SiteURL                string
	PublicMode             bool
	PublicModeExpiryDelay  int64
	UseTempRedirect        bool
	Password               string
	HashAlgorithm          string
	APIKey                 string
	SlugStyle              string
	SlugLength             int
	TryLongerSlug          bool
	AllowCapitalLetters    bool
	CustomLandingDirectory string
	UseWALMode             bool
	EnsureACID             bool
	FrontendPageSize       int
}

func Load() Config {
	if err := loadDotEnv(envString("env_file", ".env")); err != nil {
		log.Printf("could not load .env file: %v", err)
	}

	cfg := Config{
		ListenAddress:          envString("listen_address", "0.0.0.0"),
		Port:                   envInt("port", 4567),
		DBPath:                 envStringAny([]string{"db_url", "database", "db_path"}, "urls.sqlite"),
		RedisURL:               strings.TrimSpace(os.Getenv("redis_url")),
		RedisCacheKeyPrefix:    envString("redis_cache_key_prefix", "smoll-url:redirect:"),
		RedisCacheTimeoutMS:    minIntPositive(envInt("redis_cache_timeout_ms", 200), 200),
		ClickQueueSize:         minIntPositive(envInt("click_queue_size", 4096), 4096),
		ClickBatchSize:         minIntPositive(envInt("click_batch_size", 200), 200),
		ClickFlushIntervalMS:   minIntPositive(envInt("click_flush_interval_ms", 500), 500),
		MaxMindDBPath:          strings.TrimSpace(os.Getenv("maxmind_db_path")),
		CacheControlHeader:     strings.TrimSpace(os.Getenv("cache_control_header")),
		DisableFrontend:        envBool("disable_frontend", false),
		PublicMode:             envBool("public_mode", false),
		PublicModeExpiryDelay:  envInt64("public_mode_expiry_delay", 0),
		UseTempRedirect:        envBool("use_temp_redirect", false),
		Password:               strings.TrimSpace(os.Getenv("password")),
		HashAlgorithm:          normalizeHashAlgorithm(os.Getenv("hash_algorithm")),
		APIKey:                 strings.TrimSpace(os.Getenv("api_key")),
		SlugStyle:              normalizeSlugStyle(os.Getenv("slug_style")),
		SlugLength:             maxInt(envInt("slug_length", 8), 4),
		TryLongerSlug:          envBool("try_longer_slug", false),
		AllowCapitalLetters:    envBool("allow_capital_letters", false),
		CustomLandingDirectory: strings.TrimSpace(os.Getenv("custom_landing_directory")),
		UseWALMode:             envBool("use_wal_mode", false),
		EnsureACID:             envBool("ensure_acid", true),
		FrontendPageSize:       maxInt(envInt("frontend_page_size", 10), 1),
	}

	if envHas("redirect_method") {
		cfg.UseTempRedirect = strings.EqualFold(strings.TrimSpace(os.Getenv("redirect_method")), "TEMPORARY")
	}

	cfg.SiteURL = normalizeSiteURL(os.Getenv("site_url"))

	log.Printf("listening on %s:%d", cfg.ListenAddress, cfg.Port)
	log.Printf("db path: %s", cfg.DBPath)
	if cfg.RedisURL != "" {
		log.Printf("redis cache configured (timeout: %dms)", cfg.RedisCacheTimeoutMS)
	} else {
		log.Printf("redis cache disabled (redis_url not set)")
	}
	log.Printf("click queue configured (size: %d, batch: %d, flush: %dms)", cfg.ClickQueueSize, cfg.ClickBatchSize, cfg.ClickFlushIntervalMS)
	if cfg.MaxMindDBPath != "" {
		log.Printf("maxmind lookup configured (db: %s)", cfg.MaxMindDBPath)
	} else {
		log.Printf("maxmind lookup disabled (maxmind_db_path not set)")
	}
	if cfg.DisableFrontend {
		log.Printf("frontend disabled")
	}
	if cfg.PublicMode {
		log.Printf("public mode enabled (expiry cap: %d seconds)", cfg.PublicModeExpiryDelay)
	}
	if cfg.UseTempRedirect {
		log.Printf("redirect mode: temporary (307)")
	} else {
		log.Printf("redirect mode: permanent (308)")
	}

	return cfg
}

func loadDotEnv(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if k == "" {
			continue
		}
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}

		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}

	return s.Err()
}

func envHas(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func envString(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envStringAny(keys []string, fallback string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envInt64(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, ok := parseBoolish(v)
	if !ok {
		return fallback
	}
	return b
}

func parseBoolish(v string) (bool, bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "1", "true", "yes", "on", "enable", "enabled":
		return true, true
	case "0", "false", "no", "off", "disable", "disabled":
		return false, true
	default:
		return false, false
	}
}

func normalizeSiteURL(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
	}
	return strings.TrimRight(v, "/")
}

func normalizeSlugStyle(raw string) string {
	v := strings.TrimSpace(raw)
	if strings.EqualFold(v, "UID") {
		return "UID"
	}
	return "Pair"
}

func normalizeHashAlgorithm(raw string) string {
	v := strings.TrimSpace(raw)
	if strings.EqualFold(v, "Argon2") {
		return "Argon2"
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minIntPositive(v int, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}
