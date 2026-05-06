package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/agent-gateway/gateway/internal/config"
	"github.com/agent-gateway/gateway/internal/gateway"
	"github.com/agent-gateway/gateway/internal/provider"
	langgraphprovider "github.com/agent-gateway/gateway/internal/provider/langgraph"
	"github.com/agent-gateway/gateway/internal/server"
	"github.com/agent-gateway/gateway/internal/store"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	logger.Info().Str("addr", cfg.Addr()).Msg("starting agent gateway")

	s, err := store.New(cfg.Store.SQLite.Path)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize store")
	}
	defer s.Close()

	gw := gateway.New(cfg, s, logger)

	for name, entry := range cfg.Providers {
		if !entry.Enabled {
			logger.Info().Str("provider", name).Msg("provider disabled, skipping")
			continue
		}

		p, err := createProvider(name)
		if err != nil {
			logger.Warn().Str("provider", name).Err(err).Msg("failed to create provider")
			continue
		}

		provCfg := provider.ProviderConfig{
			Endpoint: entry.Endpoint,
			Auth: provider.AuthConfig{
				Token:  entry.Auth.Token,
				APIKey: entry.Auth.APIKey,
			},
			Options: entry.Options,
		}

		if err := gw.RegisterProvider(p, provCfg); err != nil {
			logger.Warn().Str("provider", name).Err(err).Msg("failed to register provider")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gw.StartHealthChecks(ctx)

	srv := server.New(cfg, gw, logger)
	httpServer := &http.Server{
		Addr:    cfg.Addr(),
		Handler: srv.Router(),
	}

	go func() {
		logger.Info().Str("addr", cfg.Addr()).Msg("http server listening")
		if err := httpServer.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
			logger.Fatal().Err(err).Msg("http server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	httpServer.Shutdown(shutdownCtx)
	gw.Shutdown(shutdownCtx)

	logger.Info().Msg("agent gateway stopped")
}

func createProvider(name string) (provider.AgentProviderAdapter, error) {
	switch name {
	case "langgraph":
		return langgraphprovider.New(zerolog.Nop()), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
