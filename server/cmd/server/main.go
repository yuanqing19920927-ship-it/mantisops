package main

import (
	"flag"
	"log"
	"os"

	"opsboard/server/internal/alert"
	"opsboard/server/internal/api"
	"opsboard/server/internal/collector"
	"opsboard/server/internal/config"
	"opsboard/server/internal/crypto"
	grpcpkg "opsboard/server/internal/grpc"
	"opsboard/server/internal/probe"
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
	groupStore := store.NewGroupStore(db)

	// VictoriaMetrics
	vmStore := store.NewVictoriaStore(cfg.Victoria.URL)

	// WebSocket Hub
	hub := ws.NewHub()

	// Metrics Collector
	mc := collector.NewMetricsCollector(vmStore, hub, serverStore)

	// Probe
	probeStore := store.NewProbeStore(db)
	prober := probe.NewProber(probeStore, vmStore, hub)
	prober.Start(cfg.Probe.Interval)
	defer prober.Stop()
	probeHandler := api.NewProbeHandler(probeStore, prober)

	// Alert system
	alertStore := store.NewAlertStore(db)
	alerter := alert.NewAlerter(alertStore, hub, mc, prober, serverStore)
	alerter.Start()
	defer alerter.Stop()
	alertHandler := api.NewAlertHandler(alertStore, alerter)

	// Asset
	assetStore := store.NewAssetStore(db)
	assetHandler := api.NewAssetHandler(assetStore)

	// Cloud stores (for DB-based instance management)
	cloudStore := store.NewCloudStore(db)
	var credStore *store.CredentialStore
	if masterKey, err := crypto.LoadKey(cfg.EncryptionKey); err == nil {
		credStore = store.NewCredentialStore(db, masterKey)
	} else {
		log.Printf("encryption key not available, credential store disabled: %v", err)
	}

	// Aliyun Cloud Collector
	var metricsProvider api.MetricsProvider
	if cfg.Aliyun.Enabled {
		ac, err := collector.NewAliyunCollector(cfg.Aliyun, vmStore, serverStore, hub, cloudStore, credStore)
		if err != nil {
			log.Printf("aliyun collector init failed: %v", err)
		} else {
			ac.MigrateFromConfig()
			ac.Start()
			defer ac.Stop()
			metricsProvider = ac
			log.Printf("aliyun collector started, %d instances", len(cfg.Aliyun.Instances))
		}
	}

	// gRPC
	handler := grpcpkg.NewHandler(serverStore, mc.Handle)
	psk := grpcpkg.NewPSKInterceptor(cfg.Server.PSKToken)
	go func() {
		if err := grpcpkg.StartPlain(cfg.Server.GRPCAddr, handler, psk); err != nil {
			log.Fatalf("gRPC error: %v", err)
		}
	}()

	// Auth
	authHandler := api.NewAuthHandler(cfg.Auth.Username, cfg.Auth.Password, cfg.Auth.JWTSecret)

	// Database (RDS)
	dbHandler := api.NewDatabaseHandler(cfg.Aliyun.RDS, vmStore)

	// Billing
	billingHandler := api.NewBillingHandler(cfg.Aliyun)

	// Groups
	groupHandler := api.NewGroupHandler(groupStore, serverStore)

	// HTTP API
	router := api.SetupRouter(api.RouterDeps{
		ServerStore:     serverStore,
		GroupStore:      groupStore,
		Hub:             hub,
		MetricsProvider: metricsProvider,
		StaticDir:       cfg.Server.StaticDir,
		ProbeHandler:    probeHandler,
		AssetHandler:    assetHandler,
		AuthHandler:     authHandler,
		DatabaseHandler: dbHandler,
		BillingHandler:  billingHandler,
		AlertHandler:    alertHandler,
		GroupHandler:    groupHandler,
	})
	log.Printf("HTTP server on %s, gRPC on %s", cfg.Server.HTTPAddr, cfg.Server.GRPCAddr)
	if err := router.Run(cfg.Server.HTTPAddr); err != nil {
		log.Fatalf("HTTP error: %v", err)
	}
}
