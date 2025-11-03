package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fiatrails/internal/config"
	"fiatrails/internal/escrow"
	"fiatrails/internal/idempotency"
	"fiatrails/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	store, err := idempotency.NewFileStore(cfg.Service.IdempotencyStorePath)
	if err != nil {
		log.Fatalf("idempotency store error: %v", err)
	}

	var escClient escrow.Client = escrow.FakeClient{}
	if cfg.Chain.PrivateKey != "" {
		ethClient, err := escrow.NewEthClient(context.Background(), escrow.EthClientConfig{
			RPCURL:             cfg.Chain.RPCURL,
			PrivateKeyHex:      cfg.Chain.PrivateKey,
			ContractMintEscrow: cfg.Deployment.Contracts.MintEscrow,
		})
		if err != nil {
			log.Fatalf("escrow client error: %v", err)
		}
		escClient = ethClient
	}

	apiServer := server.NewServer(cfg, escClient, store)

	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Service.HMACClockSkew)
	defer cancel()
	_ = apiServer.Shutdown(ctx)
}
