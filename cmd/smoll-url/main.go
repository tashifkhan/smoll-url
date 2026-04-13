package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"smoll-url/internal/config"
	"smoll-url/internal/server"
	"smoll-url/internal/store"
)

const version = "0.1.0"

func main() {
	cfg := config.Load()

	db, err := store.Open(cfg.DBPath, cfg.UseWALMode, cfg.EnsureACID)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	srv := server.New(cfg, db, version)
	srv.StartCleanupLoop()

	httpServer := &http.Server{
		Addr:              cfg.ListenAddress + ":" + itoa(cfg.Port),
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("starting smoll-url v%s", version)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Printf("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server failed: %v", err)
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	b := [20]byte{}
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
