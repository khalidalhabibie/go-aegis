package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"aegis/internal/bootstrap"
	"aegis/internal/config"
	"aegis/internal/modules/health"
	"aegis/internal/modules/reconciliation"
	"aegis/internal/modules/transfers"
	"aegis/internal/modules/wallets"
	httptransport "aegis/internal/transport/http"
	"aegis/internal/transport/http/handlers"
)

func RunAPI(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	container, err := bootstrap.NewContainer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("bootstrap container: %w", err)
	}
	defer func() {
		if closeErr := container.Close(); closeErr != nil {
			container.Logger.Error().Err(closeErr).Msg("close container")
		}
	}()

	if err := ensureTransferQueue(container.RabbitMQ, cfg.RabbitMQ); err != nil {
		return err
	}

	healthService := health.NewService(
		health.Dependencies{
			Postgres:   container.Postgres,
			Redis:      container.Redis,
			RabbitMQ:   container.RabbitMQ,
			Blockchain: container.Blockchain,
		},
		health.Metadata{
			Name:        cfg.App.Name,
			Version:     cfg.App.Version,
			Environment: cfg.App.Env,
		},
		container.StartedAt,
	)

	transferRepository := transfers.NewPostgresRepository(container.Postgres)
	transferService := transfers.NewService(transferRepository, transfers.CallbackURLPolicy{
		AllowedHosts:        cfg.CallbackURL.AllowedHosts,
		AllowPrivateTargets: cfg.CallbackURL.AllowPrivateTargets,
	}, container.Logger)
	walletRepository := wallets.NewPostgresRepository(container.Postgres)
	walletService := wallets.NewService(walletRepository, container.Logger)
	reconciliationRepository := reconciliation.NewPostgresRepository(container.Postgres)
	reconciliationService := reconciliation.NewService(
		reconciliationRepository,
		reconciliation.NewPlaceholderReceiptChecker(),
		container.Logger,
	)

	server := httptransport.NewServer(
		cfg.HTTP,
		cfg.InternalAuth,
		cfg.App.Env,
		container.Logger,
		handlers.NewHealthHandler(healthService),
		handlers.NewTransferHandler(transferService),
		handlers.NewWalletHandler(walletService),
		handlers.NewReconciliationHandler(reconciliationService),
	)
	serverErrors := make(chan error, 1)

	go func() {
		container.Logger.Info().Str("addr", server.Addr).Msg("api server started")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("run http server: %w", err)
	case <-ctx.Done():
		container.Logger.Info().Msg("api shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	return nil
}
