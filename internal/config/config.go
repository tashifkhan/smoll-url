package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress          string
	Port                   int
	DBPath                 string
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
	fileCfg := loadJSONConfig(envString("config_file", "config.json"))

	cfg := Config{
		ListenAddress:          envOrFileString(fileCfg, "listen_address", "0.0.0.0"),
		Port:                   envOrFileInt(fileCfg, "port", 4567),
		DBPath:                 envOrFileStringAny(fileCfg, []string{"db_url", "database", "db_path"}, "urls.sqlite"),
		CacheControlHeader:     envOrFileString(fileCfg, "cache_control_header", ""),
		DisableFrontend:        envOrFileBool(fileCfg, "disable_frontend", false),
		PublicMode:             envOrFileBool(fileCfg, "public_mode", false),
		PublicModeExpiryDelay:  envOrFileInt64(fileCfg, "public_mode_expiry_delay", 0),
		UseTempRedirect:        envOrFileBool(fileCfg, "use_temp_redirect", false),
		Password:               envOrFileString(fileCfg, "password", ""),
		HashAlgorithm:          normalizeHashAlgorithm(envOrFileString(fileCfg, "hash_algorithm", "")),
		APIKey:                 envOrFileString(fileCfg, "api_key", ""),
		SlugStyle:              normalizeSlugStyle(envOrFileString(fileCfg, "slug_style", "")),
		SlugLength:             maxInt(envOrFileInt(fileCfg, "slug_length", 8), 4),
		TryLongerSlug:          envOrFileBool(fileCfg, "try_longer_slug", false),
		AllowCapitalLetters:    envOrFileBool(fileCfg, "allow_capital_letters", false),
		CustomLandingDirectory: envOrFileString(fileCfg, "custom_landing_directory", ""),
		UseWALMode:             envOrFileBool(fileCfg, "use_wal_mode", false),
		EnsureACID:             envOrFileBool(fileCfg, "ensure_acid", true),
		FrontendPageSize:       maxInt(envOrFileInt(fileCfg, "frontend_page_size", 10), 1),
	}

	if envHas("redirect_method") {
		cfg.UseTempRedirect = strings.EqualFold(strings.TrimSpace(os.Getenv("redirect_method")), "TEMPORARY")
	} else if v, ok := fileString(fileCfg, "redirect_method"); ok {
		cfg.UseTempRedirect = strings.EqualFold(v, "TEMPORARY")
	}

	cfg.SiteURL = normalizeSiteURL(envOrFileString(fileCfg, "site_url", ""))

	log.Printf("listening on %s:%d", cfg.ListenAddress, cfg.Port)
	log.Printf("db path: %s", cfg.DBPath)
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

func loadJSONConfig(path string) map[string]any {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("could not read config file %q: %v", path, err)
		}
		return nil
	}

	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		log.Printf("invalid JSON in config file %q: %v", path, err)
		return nil
	}

	return out
}

func envOrFileString(cfg map[string]any, key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	if v, ok := fileString(cfg, key); ok {
		return v
	}
	return fallback
}

func envOrFileStringAny(cfg map[string]any, keys []string, fallback string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	for _, key := range keys {
		if v, ok := fileString(cfg, key); ok {
			return v
		}
	}
	return fallback
}

func envOrFileInt(cfg map[string]any, key string, fallback int) int {
	if v, ok := envIntMaybe(key); ok {
		return v
	}
	if v, ok := fileInt(cfg, key); ok {
		return v
	}
	return fallback
}

func envOrFileInt64(cfg map[string]any, key string, fallback int64) int64 {
	if v, ok := envInt64Maybe(key); ok {
		return v
	}
	if v, ok := fileInt64(cfg, key); ok {
		return v
	}
	return fallback
}

func envOrFileBool(cfg map[string]any, key string, fallback bool) bool {
	if v, ok := envBoolMaybe(key); ok {
		return v
	}
	if v, ok := fileBool(cfg, key); ok {
		return v
	}
	return fallback
}

func envHas(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func envIntMaybe(key string) (int, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return 0, false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func envInt64Maybe(key string) (int64, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return 0, false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func envBoolMaybe(key string) (bool, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return false, false
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return false, false
	}
	b, ok := parseBoolish(v)
	if !ok {
		return false, false
	}
	return b, true
}

func fileString(cfg map[string]any, key string) (string, bool) {
	if cfg == nil {
		return "", false
	}
	v, ok := cfg[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func fileInt(cfg map[string]any, key string) (int, bool) {
	if cfg == nil {
		return 0, false
	}
	v, ok := cfg[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func fileInt64(cfg map[string]any, key string) (int64, bool) {
	if cfg == nil {
		return 0, false
	}
	v, ok := cfg[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func fileBool(cfg map[string]any, key string) (bool, bool) {
	if cfg == nil {
		return false, false
	}
	v, ok := cfg[key]
	if !ok {
		return false, false
	}
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		return parseBoolish(t)
	default:
		return false, false
	}
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

func envString(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
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

func envBoolTrue(key string) bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(key)), "True")
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
