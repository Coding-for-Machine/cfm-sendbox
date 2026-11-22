// Command sendbox-api is the code execution API of cfm-sendbox: a Go port of
// the Piston v2 REST + WebSocket API built on Fiber v3.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/api"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/config"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/globals"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/job"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/pkgman"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/runtime"
)

// bodyLimit matches the 100kb JSON limit of the original Express server.
const bodyLimit = 100 * 1024

// shutdownTimeout is how long in-flight requests get to finish on SIGTERM.
const shutdownTimeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	setupLogging(cfg.LogLevel)
	slog.Info("configuration successfully loaded")

	pkgRoot := filepath.Join(cfg.DataDirectory, globals.PackagesDir)
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		return fmt.Errorf("create data directory %s: %w", pkgRoot, err)
	}

	reg := runtime.NewRegistry(cfg, globals.Platform())
	pm := pkgman.New(cfg, reg)
	slog.Info("loading packages")
	if err := pm.LoadInstalled(); err != nil {
		return fmt.Errorf("load packages: %w", err)
	}

	app := fiber.New(fiber.Config{
		AppName:      "cfm-sendbox v" + globals.Version,
		BodyLimit:    bodyLimit,
		ErrorHandler: api.ErrorHandler,
	})
	app.Use(recover.New())

	app.Get("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "cfm-sendbox v" + globals.Version})
	})
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok") })
	if len(cfg.APIKeys) > 0 {
		slog.Info("API key authentication enabled", "keys", len(cfg.APIKeys))
	}
	app.Use(api.KeyAuth(cfg.APIKeys, "/", "/healthz"))
	api.New(reg, pm, job.NewRunner(cfg)).Register(app)
	app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Not Found"})
	})

	// SIGINT/SIGTERM trigger a graceful shutdown through GracefulContext.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("starting API server", "address", cfg.BindAddress)
	err = app.Listen(cfg.BindAddress, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
		ShutdownTimeout:       shutdownTimeout,
	})
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

func setupLogging(level string) {
	var l slog.Level
	switch level {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	case "NONE":
		l = slog.Level(100)
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}
