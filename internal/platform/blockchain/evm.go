package blockchain

import (
	"context"
	"fmt"

	"aegis/internal/config"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
)

type Adapter struct {
	client  *ethclient.Client
	enabled bool
}

func New(ctx context.Context, cfg config.BlockchainConfig, log zerolog.Logger) (*Adapter, error) {
	if cfg.RPCURL == "" {
		log.Warn().Msg("evm adapter disabled because EVM_RPC_URL is empty")
		return &Adapter{}, nil
	}

	client, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("connect evm rpc: %w", err)
	}

	if cfg.ChainID > 0 {
		chainID, err := client.ChainID(ctx)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("fetch evm chain id: %w", err)
		}

		if chainID.Int64() != cfg.ChainID {
			client.Close()
			return nil, fmt.Errorf("unexpected evm chain id: got %d want %d", chainID.Int64(), cfg.ChainID)
		}
	}

	return &Adapter{
		client:  client,
		enabled: true,
	}, nil
}

func (a *Adapter) Enabled() bool {
	return a != nil && a.enabled
}

func (a *Adapter) Status(ctx context.Context) error {
	if !a.Enabled() {
		return nil
	}

	if _, err := a.client.BlockNumber(ctx); err != nil {
		return fmt.Errorf("request latest block: %w", err)
	}

	return nil
}

func (a *Adapter) Close() error {
	if !a.Enabled() {
		return nil
	}

	a.client.Close()
	return nil
}
