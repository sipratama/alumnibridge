package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/config"
	"github.com/sipratama/alumnibridge/apps/api/internal/httpserver"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identityhttp "github.com/sipratama/alumnibridge/apps/api/internal/identity/http"
	identitypostgres "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	db, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database configuration error", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	verifier, err := auth.NewJWTVerifier(ctx, cfg.OIDCJWKSURL, cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCClientID)
	if err != nil {
		logger.Error("OIDC verifier initialization failed", "error", err)
		os.Exit(1)
	}
	service := identity.NewService(identitypostgres.New(db))
	router := httpserver.New(db, logger)
	identityhttp.Register(router, service, verifier)
	server := &http.Server{Addr: cfg.Address(), Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown failed", "error", err)
		}
	}()
	logger.Info("starting api", "address", cfg.Address())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
