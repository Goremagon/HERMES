package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"openvoice/internal/auth"
	"openvoice/internal/database"
	"openvoice/internal/realtime"
)

const (
	defaultAddr         = ":8080"
	dbPath              = "data/openvoice.db"
	embedPath           = "cmd/server/dist"
	sessionCookieName   = "openvoice_session"
	sessionDuration     = 24 * time.Hour
	requestTimeout      = 3 * time.Second
	minimumPasswordSize = 8
	maxUploadSize       = 10 << 20
	uploadDir           = "uploads"
)

var (
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9]{3,20}$`)
	channelRegex  = regexp.MustCompile(`^[a-zA-Z0-9 _-]{1,30}$`)
)

//go:embed cmd/server/dist/*
var embeddedDist embed.FS

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

type channel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type meResponse struct {
	User User `json:"user"`
}

type channelsResponse struct {
	Channels []channel `json:"channels"`
}

type createChannelRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type updateProfileRequest struct {
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

type publicUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Online    bool   `json:"online"`
}

type application struct {
	db  *sql.DB
	hub *realtime.Hub
}

func main() {
	db, err := database.InitDB(dbPath)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		log.Fatalf("create uploads directory: %v", err)
	}

	distFS, err := fs.Sub(embeddedDist, embedPath)
	if err != nil {
		log.Fatalf("frontend assets unavailable: %v", err)
	}

	a := &application{db: db, hub: realtime.NewHub(db)}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/api/register", a.handleRegister)
	mux.HandleFunc("/api/login", a.handleLogin)
	mux.HandleFunc("/api/logout", a.handleLogout)
	mux.HandleFunc("/api/me", a.handleMe)
	mux.Handle("/api/users", a.authMiddleware(http.HandlerFunc(a.handleListUsers)))
	mux.Handle("/api/channels", a.authMiddleware(http.HandlerFunc(a.handleChannels)))
	mux.Handle("/api/ws", a.authMiddleware(http.HandlerFunc(a.handleWebSocket)))
	mux.Handle("/api/upload", a.authMiddleware(http.HandlerFunc(a.handleUpload)))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))
	mux.Handle("/", spaHandler(distFS))

	srv := &http.Server{
		Addr:         defaultAddr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("openvoice server listening on %s", defaultAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped unexpectedly: %v", err)
	}
}

func (a *application) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	dbStatus := "connected"
	if err := a.db.PingContext(ctx); err != nil {
		dbStatus = "disconnected"
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "db": dbStatus})
}

func (a *application) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req auth.RegisterRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if !usernameRegex.MatchString(req.Username) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username must be alphanumeric and 3-20 characters"})
		return
	}
	if len(req.Password) < minimumPasswordSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	res, err := a.db.ExecContext(ctx, `INSERT INTO users (username, password_hash) VALUES (?, ?)`, req.Username, hash)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to register user"})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch user id"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"user": User{ID: id, Username: req.Username, AvatarURL: ""}})
}

func (a *application) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req auth.LoginRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if !usernameRegex.MatchString(req.Username) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid username or password"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	var user User
	var passwordHash string
	err := a.db.QueryRowContext(ctx, `SELECT id, username, COALESCE(avatar_url, ''), password_hash FROM users WHERE username = ?`, req.Username).Scan(&user.ID, &user.Username, &user.AvatarURL, &passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to login"})
		return
	}

	if err := auth.ComparePassword(req.Password, passwordHash); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		return
	}

	token, err := auth.GenerateSessionToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
		return
	}

	expiresAt := time.Now().Add(sessionDuration).UTC()
	if _, err := a.db.ExecContext(ctx, `INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, token, user.ID, expiresAt.Format(time.RFC3339)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save session"})
		return
	}

	setSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *application) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		_, _ = a.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, cookie.Value)
	}

	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *application) handleMe(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user, err := a.userFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		writeJSON(w, http.StatusOK, meResponse{User: user})
	case http.MethodPut:
		a.handleUpdateProfile(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (a *application) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user, err := a.userFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req updateProfileRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.AvatarURL = strings.TrimSpace(req.AvatarURL)

	if !usernameRegex.MatchString(req.Username) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username must be alphanumeric and 3-20 characters"})
		return
	}
	if req.AvatarURL != "" && !strings.HasPrefix(req.AvatarURL, "/uploads/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar url must be an uploaded asset"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	if _, err := a.db.ExecContext(ctx, `UPDATE users SET username = ?, avatar_url = ? WHERE id = ?`, req.Username, req.AvatarURL, user.ID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update profile"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": User{ID: user.ID, Username: req.Username, AvatarURL: req.AvatarURL}})
}

func (a *application) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, `SELECT id, username, COALESCE(avatar_url, '') FROM users ORDER BY username ASC`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list users"})
		return
	}
	defer rows.Close()

	active := a.hub.ActiveUserIDs()
	users := make([]publicUser, 0)
	for rows.Next() {
		var u publicUser
		if err := rows.Scan(&u.ID, &u.Username, &u.AvatarURL); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse users"})
			return
		}
		u.Online = active[u.ID]
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to iterate users"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *application) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	user, err := a.userFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if err := a.hub.ServeWS(w, r, realtime.User{ID: user.ID, Username: user.Username}); err != nil {
		log.Printf("websocket handshake failed: %v", err)
	}
}

func (a *application) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload payload"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	if header.Size > maxUploadSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file too large (max 10MB)"})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webm": true}
	if !allowed[ext] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported file type"})
		return
	}

	randBytes := make([]byte, 6)
	if _, err := rand.Read(randBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate file name"})
		return
	}
	filename := fmt.Sprintf("%d-%x%s", time.Now().UnixNano(), randBytes, ext)
	path := filepath.Join(uploadDir, filename)

	out, err := os.Create(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save upload"})
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write upload"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": "/uploads/" + filename})
}

func (a *application) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleGetChannels(w, r)
	case http.MethodPost:
		a.handleCreateChannel(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (a *application) handleGetChannels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, `SELECT id, name, type FROM channels ORDER BY id ASC`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch channels"})
		return
	}
	defer rows.Close()

	channels := make([]channel, 0)
	for rows.Next() {
		var c channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Type); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse channels"})
			return
		}
		channels = append(channels, c)
	}

	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read channels"})
		return
	}

	writeJSON(w, http.StatusOK, channelsResponse{Channels: channels})
}

func (a *application) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req createChannelRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.TrimSpace(req.Type)
	if !channelRegex.MatchString(req.Name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel name must be 1-30 characters (letters, numbers, spaces, _ or -)"})
		return
	}
	if req.Type == "" {
		req.Type = "text"
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	res, err := a.db.ExecContext(ctx, `INSERT INTO channels (name, type) VALUES (?, ?)`, req.Name, req.Type)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "channel already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create channel"})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch channel id"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"channel": channel{ID: id, Name: req.Name, Type: req.Type}})
}

func (a *application) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.userFromRequest(r); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *application) userFromRequest(r *http.Request) (User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return User{}, fmt.Errorf("missing session cookie")
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	session, err := auth.GetSession(ctx, a.db, cookie.Value)
	if err != nil {
		return User{}, fmt.Errorf("get session: %w", err)
	}

	var avatarURL string
	if err := a.db.QueryRowContext(ctx, `SELECT COALESCE(avatar_url, '') FROM users WHERE id = ?`, session.UserID).Scan(&avatarURL); err != nil {
		return User{}, fmt.Errorf("load user profile: %w", err)
	}

	return User{ID: session.UserID, Username: session.Username, AvatarURL: avatarURL}, nil
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   auth.SessionCookieSecure,
		SameSite: auth.SessionCookieSameSite,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   auth.SessionCookieSecure,
		SameSite: auth.SessionCookieSameSite,
	})
}

func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is required")
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/uploads/") {
			http.NotFound(w, r)
			return
		}

		requested := r.URL.Path
		if requested == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		cleanPath := strings.TrimPrefix(requested, "/")
		if _, err := fs.Stat(staticFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
