package main

import (
	"flag"
	"log"
	"os"

	"opsboard/server/internal/api"
	"opsboard/server/internal/collector"
	"opsboard/server/internal/config"
	grpcpkg "opsboard/server/internal/grpc"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
)

func main() {
	cfgPath := flag.String("config", "configs/server.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// SQLite
	os.MkdirAll("data", 0755)
	db, err := store.InitSQLite(cfg.SQLite.Path)
	if err != nil {
		log.Fatalf("init sqlite: %v", err)
	}
	defer db.Close()
	serverStore := store.NewServerStore(db)

	// VictoriaMetrics
	vmStore := store.NewVictoriaStore(cfg.Victoria.URL)

	// WebSocket Hub
	hub := ws.NewHub()

	// Metrics Collector
	mc := collector.NewMetricsCollector(vmStore, hub, serverStore)

	// gRPC
	handler := grpcpkg.NewHandler(serverStore, mc.Handle)
	psk := grpcpkg.NewPSKInterceptor(cfg.Server.PSKToken)
	go func() {
		if err := grpcpkg.StartPlain(cfg.Server.GRPCAddr, handler, psk); err != nil {
			log.Fatalf("gRPC error: %v", err)
		}
	}()

	// HTTP API
	router := api.SetupRouter(serverStore, hub)
	log.Printf("HTTP server on %s, gRPC on %s", cfg.Server.HTTPAddr, cfg.Server.GRPCAddr)
	if err := router.Run(cfg.Server.HTTPAddr); err != nil {
		log.Fatalf("HTTP error: %v", err)
	}
}
