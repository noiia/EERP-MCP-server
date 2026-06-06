// Command server starts the EERP documentation MCP server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/bcrypt"

	"github.com/elnoia/eerp-mcp-server/internal/cache"
	"github.com/elnoia/eerp-mcp-server/internal/config"
	"github.com/elnoia/eerp-mcp-server/internal/indexer"
	mcpserver "github.com/elnoia/eerp-mcp-server/internal/mcp"
	"github.com/elnoia/eerp-mcp-server/internal/search"
	"github.com/elnoia/eerp-mcp-server/internal/watcher"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		cfgPath  = flag.String("config", "configs/config.yaml", "path to config YAML file")
		logLevel = flag.String("log-level", "info", "log level: debug, info, warn, error")
		stdio    = flag.Bool("stdio", false, "use stdio transport (overrides config transport)")
		genHash  = flag.String("gen-hash", "", "print a bcrypt hash for the given password and exit")
	)
	flag.Parse()

	if *genHash != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*genHash), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("bcrypt: %w", err)
		}
		fmt.Println(string(hash))
		return nil
	}

	logger := buildLogger(*logLevel)

	// Load config.
	cfg, err := loadConfig(*cfgPath, logger)
	if err != nil {
		return err
	}
	if *stdio {
		cfg.Server.Transport = "stdio"
	}

	// Bootstrap the in-memory cache and search engine (empty at first).
	c := cache.New()
	e := search.New(nil)

	// Build the indexer and run the initial indexing pass.
	idx := indexer.New(cfg.Docs.MkDocs, cfg.Docs.Root, c, e, logger)
	if err := idx.Index(); err != nil {
		return fmt.Errorf("initial index: %w", err)
	}

	// Create the MCP server.
	srv := mcpserver.New(cfg, c, e, logger)

	// Context that is cancelled on SIGINT / SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start the file watcher in the background.
	stopWatcher := make(chan struct{})
	go func() {
		w := watcher.New(cfg.Docs.Root, cfg.Docs.MkDocs, idx.Index, logger)
		if err := w.Run(stopWatcher); err != nil {
			logger.Error("watcher stopped", "error", err)
		}
	}()

	// Run the appropriate transport.
	var runErr error
	switch cfg.Server.Transport {
	case "stdio":
		runErr = srv.RunStdio(ctx)
	default:
		runErr = srv.RunHTTP(ctx)
	}

	close(stopWatcher)
	return runErr
}

func loadConfig(path string, logger *slog.Logger) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		logger.Warn("config not found, using defaults", "path", path, "error", err)
		cfg = config.Default()
	}
	return cfg, nil
}

func buildLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
