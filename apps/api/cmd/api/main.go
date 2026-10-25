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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/config"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
	directoryhttp "github.com/sipratama/alumnibridge/apps/api/internal/directory/http"
	directorypostgres "github.com/sipratama/alumnibridge/apps/api/internal/directory/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/httpserver"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identityhttp "github.com/sipratama/alumnibridge/apps/api/internal/identity/http"
	identitypostgres "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
	profileshttp "github.com/sipratama/alumnibridge/apps/api/internal/profiles/http"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles/objectstorage"
	profilespostgres "github.com/sipratama/alumnibridge/apps/api/internal/profiles/postgres"
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
	// Directory overload cannot consume every connection used by owner APIs and
	// readiness checks. Account for both pools in the deployment connection budget.
	searchConfig := db.Config()
	searchConfig.MaxConns = int32(cfg.DirectoryMaxConcurrent)
	searchConfig.MinConns = 0
	searchConfig.ConnConfig.RuntimeParams["application_name"] = "alumnibridge-directory"
	searchDB, err := pgxpool.NewWithConfig(context.Background(), searchConfig)
	if err != nil {
		logger.Error("directory database configuration error")
		os.Exit(1)
	}
	defer searchDB.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	identityStore := identitypostgres.New(db, cfg.OIDCIssuer)
	verifier, err := auth.NewJWTVerifier(ctx, cfg.OIDCJWKSURL, cfg.OIDCIssuer, cfg.OIDCAudience, cfg.OIDCClientID, identityStore)
	if err != nil {
		logger.Error("OIDC verifier initialization failed", "error", err)
		os.Exit(1)
	}
	storageCtx, storageCancel := context.WithTimeout(ctx, 20*time.Second)
	photoStorage, err := objectstorage.New(storageCtx, cfg.StorageEndpoint, cfg.StorageAccessKey, cfg.StorageSecretKey, cfg.StorageBucket, cfg.StorageSecure)
	storageCancel()
	if err != nil {
		logger.Error("object storage initialization failed")
		os.Exit(1)
	}
	profileStore := profilespostgres.New(db)
	profileService := profiles.New(profileStore)
	publication := profiles.NewPublishing(profileStore, photoStorage)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				if err := publication.Cleanup(cleanupCtx); err != nil {
					logger.Warn("photo cleanup retry pending")
				}
				cancel()
			}
		}
	}()
	cursor, err := directory.NewCursor(cfg.DirectoryCursorKey)
	if err != nil {
		logger.Error("directory cursor initialization failed")
		os.Exit(1)
	}
	directoryService := directory.New(directorypostgres.NewWithTimeout(searchDB, cfg.DirectoryStatementTimeout), cursor)
	discovery := profiles.NewDiscovery(profileStore)
	service := identity.NewService(identityStore)
	router := httpserver.New(db, logger)
	router.Route("/api/v1", func(r chi.Router) {
		identityhttp.RegisterRoutes(r, service, verifier)
		directoryhttp.RegisterRoutesWithLimits(r, directoryService, directoryhttp.Limits{
			RequestTimeout: cfg.DirectoryRequestTimeout, MaxConcurrent: cfg.DirectoryMaxConcurrent,
			RatePerMinute: cfg.DirectoryRatePerMinute, Burst: cfg.DirectoryBurst,
			MaxClients: cfg.DirectoryMaxClients, ClientTTL: cfg.DirectoryClientTTL,
		}, logger)
		profileshttp.RegisterDiscoveryRoutes(r, profileService, discovery, service, verifier)
		profileshttp.RegisterRoutes(r, profileService, service, verifier)
		profileshttp.RegisterPublishingRoutes(r, profileService, publication, service, verifier)
	})
	server := &http.Server{Addr: cfg.Address(), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
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
