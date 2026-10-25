package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/aalvaropc/upkeep/internal/events"
)

//go:embed ui
var uiFiles embed.FS

const DefaultKeepalive = 15 * time.Second

type Options struct {
	Hub       *events.Hub
	Version   string
	Keepalive time.Duration
	Logger    *slog.Logger
	Done      <-chan struct{}
}

type server struct {
	opts Options
}

func New(opts Options) http.Handler {
	if opts.Keepalive <= 0 {
		opts.Keepalive = DefaultKeepalive
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.Hub == nil {
		opts.Hub = events.NewHub(1)
	}
	ui, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		panic(err)
	}
	s := &server{opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.events)
	mux.Handle("GET /", noCache(http.FileServerFS(ui)))
	return securityHeaders(mux)
}

func (s *server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.opts.Version})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}
