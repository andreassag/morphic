package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/exterex/morphic/internal/database"
	"github.com/exterex/morphic/internal/shared"
	"github.com/exterex/morphic/web"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	host := flag.String("host", "0.0.0.0", "Host to bind to")
	port := flag.Int("port", 8001, "Port to listen on")
	dbURL := flag.String("database-url", "", "PostgreSQL connection string (e.g. postgres://morphic:morphic@localhost:5432/morphic)")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Morphic version %s (%s/%s)\n", shared.Version, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// Setup root context with SIGINT / SIGTERM notification
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Resolve database URL from flag or environment
	resolvedDBURL := *dbURL
	if resolvedDBURL == "" {
		resolvedDBURL = os.Getenv("DATABASE_URL")
	}

	var pool *pgxpool.Pool
	if resolvedDBURL != "" {
		slog.Info("connecting to PostgreSQL database...")
		var err error
		pool, err = database.Connect(ctx, database.DefaultConfig(resolvedDBURL))
		if err != nil {
			slog.Error("failed to connect to PostgreSQL database", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		slog.Info("PostgreSQL database connected and schema migrated successfully")
	} else {
		slog.Warn("no DATABASE_URL provided; running in standalone mode without persistent caching")
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	url := fmt.Sprintf("http://localhost:%d", *port)

	router, err := web.SetupRouter(ctx, pool)
	if err != nil {
		slog.Error("failed to configure router", "err", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("Morphic starting", "version", shared.Version, "addr", addr, "url", url)
		if !*noBrowser && runtime.GOOS != "linux" {
			go openBrowser(url)
		}
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()
	slog.Info("shutting down server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "err", err)
	} else {
		slog.Info("server exited cleanly")
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
