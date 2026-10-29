package web

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aalvaropc/upkeep/internal/demo"
	"github.com/aalvaropc/upkeep/internal/events"
)

//go:embed all:dist
var distFiles embed.FS

const DefaultKeepalive = 15 * time.Second

const notBuilt = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Upkeep</title></head>
<body>
<h1>Upkeep</h1>
<p>The dashboard has not been built into this binary. Run <code>make web</code> and build again.</p>
<p>The API is running: <a href="/healthz">/healthz</a>.</p>
</body>
</html>
`

type Options struct {
	Hub       *events.Hub
	Version   string
	Keepalive time.Duration
	Logger    *slog.Logger
	Done      <-chan struct{}
	UI        fs.FS

	Demo       *demo.Manager
	Info       func() map[string]any
	TrustProxy bool
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
	if opts.UI == nil {
		opts.UI = EmbeddedUI()
	}
	s := &server{opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.events)
	if opts.Demo != nil {
		s.routeDemo(mux)
	}
	mux.Handle("GET /", dashboard(opts.UI))
	return securityHeaders(mux)
}

func EmbeddedUI() fs.FS {
	ui, err := fs.Sub(distFiles, "dist")
	if err != nil {
		panic(err)
	}
	return ui
}

func Built(ui fs.FS) bool {
	_, err := fs.Stat(ui, "index.html")
	return err == nil
}

func dashboard(ui fs.FS) http.Handler {
	if !Built(ui) {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" && !isDemoPage(r.URL.Path) {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = io.WriteString(w, notBuilt)
		})
	}
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.") {
			http.NotFound(w, r)
			return
		}
		if isDemoPage(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, ui, "index.html")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func isDemoPage(path string) bool {
	return path == "/demo" || path == "/demo/"
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
