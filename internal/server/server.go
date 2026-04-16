package server

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"smoll-url/internal/auth"
	"smoll-url/internal/config"
	"smoll-url/internal/slug"
	"smoll-url/internal/store"
)

//go:embed web/index.html web/static/*
var embeddedWeb embed.FS

const maxExpiryDelay = int64(157784760)

type Server struct {
	cfg      config.Config
	store    *store.Store
	sessions *auth.SessionStore
	version  string
	cache    *redirectCache
	clicks   *clickTracker

	validSlugRegex *regexp.Regexp
}

type JSONResponse struct {
	Success bool   `json:"success"`
	Error   bool   `json:"error"`
	Reason  string `json:"reason"`
}

type CreatedURL struct {
	Success    bool   `json:"success"`
	Error      bool   `json:"error"`
	ShortURL   string `json:"shorturl"`
	ExpiryTime int64  `json:"expiry_time"`
}

type LinkInfo struct {
	Success    bool   `json:"success"`
	Error      bool   `json:"error"`
	LongURL    string `json:"longurl"`
	Hits       int64  `json:"hits"`
	ExpiryTime int64  `json:"expiry_time"`
}

type backendConfig struct {
	Version               string `json:"version"`
	SiteURL               string `json:"site_url,omitempty"`
	AllowCapitalLetters   bool   `json:"allow_capital_letters"`
	PublicMode            bool   `json:"public_mode"`
	PublicModeExpiryDelay int64  `json:"public_mode_expiry_delay"`
	SlugStyle             string `json:"slug_style"`
	SlugLength            int    `json:"slug_length"`
	TryLongerSlug         bool   `json:"try_longer_slug"`
	FrontendPageSize      int    `json:"frontend_page_size"`
}

type addLinkRequest struct {
	Shortlink   string `json:"shortlink"`
	Longlink    string `json:"longlink"`
	ExpiryDelay int64  `json:"expiry_delay"`
}

type editLinkRequest struct {
	Shortlink string `json:"shortlink"`
	Longlink  string `json:"longlink"`
	ResetHits bool   `json:"reset_hits"`
}

func New(cfg config.Config, db *store.Store, version string) *Server {
	re := regexp.MustCompile(`^[a-z0-9-_]+$`)
	if cfg.AllowCapitalLetters {
		re = regexp.MustCompile(`^[A-Za-z0-9-_]+$`)
	}

	return &Server{
		cfg:      cfg,
		store:    db,
		sessions: auth.NewSessionStore(),
		version:  version,
		cache: newRedirectCache(
			cfg.RedisURL,
			cfg.RedisCacheKeyPrefix,
			time.Duration(cfg.RedisCacheTimeoutMS)*time.Millisecond,
		),
		clicks:         newClickTracker(cfg, db),
		validSlugRegex: re,
	}
}

func (s *Server) Close() error {
	return errors.Join(s.clicks.close(), s.cache.close())
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/new", s.handleAddLink)
	mux.HandleFunc("/api/all", s.handleGetAll)
	mux.HandleFunc("/api/expand", s.handleExpand)
	mux.HandleFunc("/api/edit", s.handleEditLink)
	mux.HandleFunc("/api/getconfig", s.handleGetConfig)
	mux.HandleFunc("/api/siteurl", s.handleSiteURL)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/whoami", s.handleWhoAmI)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/del/", s.handleDeleteLink)

	if !s.cfg.DisableFrontend {
		if s.cfg.CustomLandingDirectory == "" {
			webFS, err := fs.Sub(embeddedWeb, "web")
			if err != nil {
				panic(err)
			}
			mux.Handle("/static/", http.FileServer(http.FS(webFS)))
		} else {
			webFS, err := fs.Sub(embeddedWeb, "web")
			if err != nil {
				panic(err)
			}
			mux.Handle("/admin/manage/", http.StripPrefix("/admin/manage/", http.FileServer(http.FS(webFS))))
		}
	}

	mux.HandleFunc("/", s.handleRoot)

	return s.withHeaders(s.withLogging(mux))
}

func (s *Server) StartCleanupLoop() {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.store.Cleanup(); err != nil {
				log.Printf("cleanup error: %v", err)
			}
		}
	}()
}

func (s *Server) handleAddLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	body, err := readBody(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, JSONResponse{Success: false, Error: true, Reason: "Invalid request!"})
		return
	}

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	if apiResult.Success {
		shortlink, expiryTime, err := s.addLinkFromBody(body, false)
		if err != nil {
			s.handleAddLinkError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, CreatedURL{
			Success:    true,
			Error:      false,
			ShortURL:   s.fullURL(shortlink),
			ExpiryTime: expiryTime,
		})
		return
	}

	if apiResult.Error {
		writeJSON(w, http.StatusUnauthorized, apiResult)
		return
	}

	usingPublic := false
	if s.isSessionValid(r) {
		usingPublic = false
	} else if s.cfg.PublicMode {
		usingPublic = true
	} else {
		writeText(w, http.StatusUnauthorized, "Not logged in!")
		return
	}

	shortlink, _, err := s.addLinkFromBody(body, usingPublic)
	if err != nil {
		s.handleAddLinkErrorPlain(w, err)
		return
	}

	writeText(w, http.StatusCreated, shortlink)
}

func (s *Server) handleGetAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	if !apiResult.Success {
		if apiResult.Error {
			writeJSON(w, http.StatusUnauthorized, apiResult)
			return
		}
		if !s.isSessionValid(r) {
			writeText(w, http.StatusUnauthorized, "Not logged in!")
			return
		}
	}

	q := r.URL.Query()
	pageAfter := strings.TrimSpace(q.Get("page_after"))
	pageNo := parsePositiveInt64(q.Get("page_no"))
	pageSize := parsePositiveInt64(q.Get("page_size"))
	if (pageAfter != "" || pageNo > 0) && pageSize == 0 {
		pageSize = 10
	}

	rows, err := s.store.GetAll(pageAfter, pageNo, pageSize)
	if err != nil {
		writeText(w, http.StatusInternalServerError, "[]")
		return
	}

	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleExpand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	if !apiResult.Success {
		writeJSON(w, http.StatusUnauthorized, apiResult)
		return
	}

	body, err := readBody(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, JSONResponse{Success: false, Error: true, Reason: "Malformed request!"})
		return
	}
	shortlink := strings.TrimSpace(string(body))

	longURL, hits, expiryTime, err := s.store.FindURL(shortlink)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, JSONResponse{Success: false, Error: true, Reason: "The shortlink does not exist on the server!"})
			return
		}
		writeJSON(w, http.StatusBadRequest, JSONResponse{Success: false, Error: true, Reason: "Something went wrong when finding the link."})
		return
	}

	writeJSON(w, http.StatusOK, LinkInfo{Success: true, Error: false, LongURL: longURL, Hits: hits, ExpiryTime: expiryTime})
}

func (s *Server) handleEditLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w)
		return
	}

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	if !apiResult.Success && !s.isSessionValid(r) {
		writeJSON(w, http.StatusUnauthorized, apiResult)
		return
	}

	body, err := readBody(r.Body)
	if err != nil {
		s.writeEditError(w, true, "Malformed request!")
		return
	}

	var req editLinkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeEditError(w, true, "Malformed request!")
		return
	}

	req.Shortlink = strings.TrimSpace(req.Shortlink)
	req.Longlink = strings.TrimSpace(req.Longlink)
	if req.Shortlink == "" || !s.validSlugRegex.MatchString(req.Shortlink) {
		s.writeEditError(w, true, "Invalid shortlink!")
		return
	}
	if req.Longlink == "" {
		s.writeEditError(w, true, "Malformed request!")
		return
	}

	err = s.store.EditLink(req.Shortlink, req.Longlink, req.ResetHits)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.cache.delete(req.Shortlink)
			s.writeEditError(w, true, "The shortlink was not found, and could not be edited.")
			return
		}
		s.writeEditError(w, false, "Something went wrong when editing the link.")
		return
	}

	if _, _, expiryTime, err := s.store.FindURL(req.Shortlink); err == nil {
		s.cache.set(req.Shortlink, req.Longlink, expiryTime)
	} else {
		s.cache.delete(req.Shortlink)
	}

	if apiResult.Success {
		writeJSON(w, http.StatusCreated, JSONResponse{Success: true, Error: false, Reason: "Edit was successful."})
		return
	}

	writeText(w, http.StatusCreated, "Edit was successful.")
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	if !(apiResult.Success || s.isSessionValid(r) || s.cfg.PublicMode) {
		writeJSON(w, http.StatusUnauthorized, apiResult)
		return
	}

	writeJSON(w, http.StatusOK, backendConfig{
		Version:               s.version,
		SiteURL:               s.cfg.SiteURL,
		AllowCapitalLetters:   s.cfg.AllowCapitalLetters,
		PublicMode:            s.cfg.PublicMode,
		PublicModeExpiryDelay: s.cfg.PublicModeExpiryDelay,
		SlugStyle:             s.cfg.SlugStyle,
		SlugLength:            s.cfg.SlugLength,
		TryLongerSlug:         s.cfg.TryLongerSlug,
		FrontendPageSize:      s.cfg.FrontendPageSize,
	})
}

func (s *Server) handleSiteURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	if s.cfg.SiteURL == "" {
		writeText(w, http.StatusOK, "unset")
		return
	}
	writeText(w, http.StatusOK, s.cfg.SiteURL)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	writeText(w, http.StatusOK, "smoll-url v"+s.version)
}

func (s *Server) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	role := "nobody"
	if apiResult.Success || s.isSessionValid(r) {
		role = "admin"
	} else if s.cfg.PublicMode {
		role = "public"
	}

	writeText(w, http.StatusOK, role)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	if strings.TrimSpace(s.cfg.Password) == "" {
		if s.cfg.APIKey != "" {
			writeText(w, http.StatusUnauthorized, "Password login is disabled. Use API key authentication.")
			return
		}
		writeText(w, http.StatusUnauthorized, "Password login is disabled. Set 'password' in config.")
		return
	}

	body, err := readBody(r.Body)
	if err != nil {
		writeText(w, http.StatusBadRequest, "Wrong password!")
		return
	}
	password := string(body)

	if !auth.IsPasswordValid(password, s.cfg) {
		if s.cfg.APIKey != "" {
			writeJSON(w, http.StatusUnauthorized, JSONResponse{Success: false, Error: true, Reason: "Wrong password!"})
			return
		}
		writeText(w, http.StatusUnauthorized, "Wrong password!")
		return
	}

	token := s.sessions.NewToken()
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 14,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false,
	})

	if s.cfg.APIKey != "" {
		writeJSON(w, http.StatusOK, JSONResponse{Success: true, Error: false, Reason: "Correct password!"})
		return
	}
	writeText(w, http.StatusOK, "Correct password!")
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		writeText(w, http.StatusUnauthorized, "You don't seem to be logged in.")
		return
	}

	s.sessions.DeleteToken(cookie.Value)
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   false,
	})

	writeText(w, http.StatusOK, "Logged out!")
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w)
		return
	}

	shortlink := strings.TrimPrefix(r.URL.Path, "/api/del/")
	shortlink = strings.TrimSpace(shortlink)

	apiResult := auth.IsAPIAuthorized(r, s.cfg)
	if !apiResult.Success {
		if apiResult.Error {
			writeJSON(w, http.StatusUnauthorized, apiResult)
			return
		}
		if !s.isSessionValid(r) {
			writeText(w, http.StatusUnauthorized, "Not logged in!")
			return
		}
	}

	if !s.validSlugRegex.MatchString(shortlink) {
		if apiResult.Success {
			writeJSON(w, http.StatusNotFound, JSONResponse{Success: false, Error: true, Reason: "The shortlink is invalid."})
			return
		}
		writeText(w, http.StatusNotFound, "Not found!")
		return
	}

	err := s.store.DeleteLink(shortlink)
	if err != nil {
		s.cache.delete(shortlink)
		if apiResult.Success {
			writeJSON(w, http.StatusNotFound, JSONResponse{Success: false, Error: true, Reason: "The shortlink was not found, and could not be deleted."})
			return
		}
		writeText(w, http.StatusNotFound, "Not found!")
		return
	}
	s.cache.delete(shortlink)

	if apiResult.Success {
		writeJSON(w, http.StatusOK, JSONResponse{Success: true, Error: false, Reason: "Deleted " + shortlink})
		return
	}

	writeText(w, http.StatusOK, "Deleted "+shortlink)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		write404(w)
		return
	}

	if !s.cfg.DisableFrontend {
		if s.cfg.CustomLandingDirectory != "" {
			if r.URL.Path == "/admin/manage" {
				http.Redirect(w, r, "/admin/manage/", http.StatusTemporaryRedirect)
				return
			}

			if s.serveCustomLandingIfExists(w, r) {
				return
			}
		} else if r.URL.Path == "/" {
			content, err := embeddedWeb.ReadFile("web/index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(content)
				return
			}
		}
	}

	if r.URL.Path == "/" {
		write404(w)
		return
	}

	shortlink := strings.TrimPrefix(r.URL.Path, "/")
	if strings.Contains(shortlink, "/") || shortlink == "" || !s.validSlugRegex.MatchString(shortlink) {
		write404(w)
		return
	}

	if cachedURL, ok := s.cache.get(shortlink); ok {
		s.enqueueClick(shortlink, r)
		s.redirectToURL(w, r, cachedURL)
		return
	}

	longURL, _, expiryTime, err := s.store.FindURL(shortlink)
	if err != nil {
		write404(w)
		return
	}
	s.cache.set(shortlink, longURL, expiryTime)
	s.enqueueClick(shortlink, r)
	s.redirectToURL(w, r, longURL)
}

func (s *Server) enqueueClick(shortlink string, r *http.Request) {
	s.clicks.enqueue(clickQueueItem{
		Shortlink: shortlink,
		ClickedAt: time.Now().UTC().Unix(),
		IP:        clientIPFromRequest(r),
		UserAgent: strings.TrimSpace(r.UserAgent()),
		Referer:   strings.TrimSpace(r.Referer()),
	})
}

func (s *Server) redirectToURL(w http.ResponseWriter, r *http.Request, longURL string) {
	status := http.StatusPermanentRedirect
	if s.cfg.UseTempRedirect {
		status = http.StatusTemporaryRedirect
	}
	http.Redirect(w, r, longURL, status)
}

func (s *Server) addLinkFromBody(body []byte, usingPublicMode bool) (string, int64, error) {
	var req addLinkRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", 0, store.ErrBadRequest
	}

	req.Shortlink = strings.TrimSpace(req.Shortlink)
	req.Longlink = strings.TrimSpace(req.Longlink)
	if req.Longlink == "" {
		return "", 0, store.ErrBadRequest
	}

	if usingPublicMode && s.cfg.PublicModeExpiryDelay > 0 {
		if req.ExpiryDelay <= 0 {
			req.ExpiryDelay = s.cfg.PublicModeExpiryDelay
		} else if req.ExpiryDelay > s.cfg.PublicModeExpiryDelay {
			req.ExpiryDelay = s.cfg.PublicModeExpiryDelay
		}
	}

	if req.ExpiryDelay < 0 {
		req.ExpiryDelay = 0
	}
	if req.ExpiryDelay > maxExpiryDelay {
		req.ExpiryDelay = maxExpiryDelay
	}

	shortProvided := req.Shortlink != ""
	if !shortProvided {
		req.Shortlink = slug.Generate(s.cfg.SlugStyle, s.cfg.SlugLength, s.cfg.AllowCapitalLetters)
	} else if !s.validSlugRegex.MatchString(req.Shortlink) {
		return "", 0, store.ErrBadRequest
	}

	expiryTime, err := s.store.AddLink(req.Shortlink, req.Longlink, req.ExpiryDelay)
	if err == nil {
		s.cache.set(req.Shortlink, req.Longlink, expiryTime)
		return req.Shortlink, expiryTime, nil
	}

	if !errors.Is(err, store.ErrConflict) || shortProvided {
		return "", 0, err
	}

	retryLen := s.cfg.SlugLength
	if s.cfg.SlugStyle == "UID" && s.cfg.TryLongerSlug {
		retryLen += 4
	}

	req.Shortlink = slug.Generate(s.cfg.SlugStyle, retryLen, s.cfg.AllowCapitalLetters)
	expiryTime, err = s.store.AddLink(req.Shortlink, req.Longlink, req.ExpiryDelay)
	if err != nil {
		return "", 0, err
	}
	s.cache.set(req.Shortlink, req.Longlink, expiryTime)

	return req.Shortlink, expiryTime, nil
}

func (s *Server) handleAddLinkError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrBadRequest) {
		writeJSON(w, http.StatusConflict, JSONResponse{Success: false, Error: true, Reason: "Invalid request!"})
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeJSON(w, http.StatusConflict, JSONResponse{Success: false, Error: true, Reason: "Short URL is already in use!"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, JSONResponse{Success: false, Error: true, Reason: "Something went wrong when adding the link."})
}

func (s *Server) handleAddLinkErrorPlain(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrBadRequest) {
		writeText(w, http.StatusConflict, "Invalid request!")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeText(w, http.StatusConflict, "Short URL is already in use!")
		return
	}
	writeText(w, http.StatusInternalServerError, "Something went wrong when adding the link.")
}

func (s *Server) writeEditError(w http.ResponseWriter, client bool, reason string) {
	if client {
		writeJSON(w, http.StatusBadRequest, JSONResponse{Success: false, Error: true, Reason: reason})
		return
	}
	writeJSON(w, http.StatusInternalServerError, JSONResponse{Success: false, Error: true, Reason: reason})
}

func (s *Server) isSessionValid(r *http.Request) bool {
	if s.cfg.Password == "" {
		return false
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return false
	}
	return s.sessions.IsValid(cookie.Value)
}

func (s *Server) fullURL(shortlink string) string {
	if s.cfg.SiteURL != "" {
		return strings.TrimRight(s.cfg.SiteURL, "/") + "/" + shortlink
	}

	protocol := "http"
	if s.cfg.Port == 443 {
		protocol = "https"
	}
	portText := ""
	if s.cfg.Port != 80 && s.cfg.Port != 443 {
		portText = ":" + strconv.Itoa(s.cfg.Port)
	}

	return protocol + "://localhost" + portText + "/" + shortlink
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.CacheControlHeader != "" {
			w.Header().Set("Cache-Control", s.cfg.CacheControlHeader)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveCustomLandingIfExists(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/" {
		indexPath := filepath.Join(s.cfg.CustomLandingDirectory, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return true
		}
		return false
	}

	cleanPath := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(cleanPath, "/")
	if rel == "" {
		return false
	}
	filePath := filepath.Join(s.cfg.CustomLandingDirectory, rel)
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}

	if info.IsDir() {
		idx := filepath.Join(filePath, "index.html")
		if _, err := os.Stat(idx); err == nil {
			http.ServeFile(w, r, idx)
			return true
		}
		return false
	}

	if ctype := mime.TypeByExtension(filepath.Ext(filePath)); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	http.ServeFile(w, r, filePath)
	return true
}

func parsePositiveInt64(raw string) int64 {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func readBody(r io.ReadCloser) ([]byte, error) {
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 1<<20))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, text)
}

func write404(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "<!doctype html><html><body><h1>404</h1><p>Not found.</p></body></html>")
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeText(w, http.StatusMethodNotAllowed, "Method not allowed")
}
