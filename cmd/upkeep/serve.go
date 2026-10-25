package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aalvaropc/upkeep/internal/buildinfo"
	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/events"
	"github.com/aalvaropc/upkeep/internal/store"
	"github.com/aalvaropc/upkeep/internal/web"
)

const (
	localSession  = "local"
	ringCapacity  = 1000
	shutdownGrace = 10 * time.Second
)

func serve(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) error {
	if len(args) > 0 {
		return fmt.Errorf("serve takes no arguments, got %q", args[0])
	}
	cfg, err := config.Load(environ)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	st, err := store.Open(ctx, filepath.Join(cfg.DataDir, "upkeep.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	hub := events.NewHub(ringCapacity)
	hub.Open(localSession)
	done := make(chan struct{})
	handler := web.New(web.Options{Hub: hub, Version: buildinfo.Version(), Logger: logger, Done: done})

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	srv.RegisterOnShutdown(func() { close(done) })

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	logger.Info("serving", "url", "http://"+ln.Addr().String(), "config", cfg)
	fmt.Fprintf(stdout, "upkeep listening on http://%s\n", ln.Addr())

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
