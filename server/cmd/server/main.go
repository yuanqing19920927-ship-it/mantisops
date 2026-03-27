package main

import (
	"flag"
	"log"
	"os"

	"opsboard/server/internal/alert"
	"opsboard/server/internal/api"
	"opsboard/server/internal/cloud"
	"opsboard/server/internal/collector"
	"opsboard/server/internal/config"
	"opsboard/server/internal/crypto"
	"opsboard/server/internal/deployer"
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

	// 1. SQLite (with foreign_keys)
	os.MkdirAll("data", 0755)
	db, err := store.InitSQLite(cfg.SQLite.Path)
	if err != nil {
		log.Fatalf("init sqlite: %v", err)
	}
	defer db.Close()

	// 2. Encryption key
	masterKey, err := crypto.EnsureKey(cfg.EncryptionKey, *cfgPath)
	if err != nil {
		log.Fatalf("encryption key: %v", err)
	}

	// 3. Stores
	serverStore := store.NewServerStore(db)
	groupStore := store.NewGroupStore(db)
	credentialStore := store.NewCredentialStore(db, masterKey)
	cloudStore := store.NewCloudStore(db)
	managedServerStore := store.NewManagedServerStore(db)

	// 4. VictoriaMetrics
	vmStore := store.NewVictoriaStore(cfg.Victoria.URL)

	// 5. WebSocket Hub
	hub := ws.NewHub()

	// 6. Metrics Collector
	mc := collector.NewMetricsCollector(vmStore, hub, serverStore)

	// 7. Cloud Manager
	cloudManager := cloud.NewManager(db, cloudStore, credentialStore, hub)

	// 8. Deployer
	binaryDir := cfg.AgentBin.BinaryDir
	if binaryDir == "" {
		binaryDir = "./build"
	}
	registerTimeout := cfg.AgentBin.RegisterTimeout
	if registerTimeout <= 0 {
		registerTimeout = 120
	}
	dep := deployer.NewDeployer(
		managedServerStore, credentialStore, serverStore, hub,
		cfg.Server.PSKToken, cfg.Server.GRPCAddr, binaryDir, registerTimeout,
	)

	// 9. Probe
	probeStore := store.NewProbeStore(db)
	prober := probe.NewProber(probeStore, vmStore, hub)
	prober.Start(cfg.Probe.Interval)
	defer prober.Stop()
	probeHandler := api.NewProbeHandler(probeStore, prober)

	// 10. Alert system
	alertStore := store.NewAlertStore(db)
	alerter := alert.NewAlerter(alertStore, hub, mc, prober, serverStore)
	alerter.Start()
	defer alerter.Stop()
	alertHandler := api.NewAlertHandler(alertStore, alerter)

	// 11. Asset
	assetStore := store.NewAssetStore(db)
	assetHandler := api.NewAssetHandler(assetStore)

	// 12. Aliyun Cloud Collector
	var metricsProvider api.MetricsProvider
	if cfg.Aliyun.Enabled {
		ac, err := collector.NewAliyunCollector(cfg.Aliyun, vmStore, serverStore, hub, cloudStore, credentialStore)
		if err != nil {
			log.Printf("aliyun collector init failed: %v", err)
		} else {
			if migratedID := ac.MigrateFromConfig(); migratedID > 0 {
				// Sync to fetch instance metadata (names, engine, spec, endpoint)
				cloudManager.Sync(migratedID)
			}
			ac.Start()
			defer ac.Stop()
			metricsProvider = ac
			log.Printf("aliyun collector started")
		}
	}

	// 13. gRPC with deployer callback
	handler := grpcpkg.NewHandler(serverStore, mc.Handle, dep.NotifyRegistered)
	psk := grpcpkg.NewPSKInterceptor(cfg.Server.PSKToken)
	go func() {
		if err := grpcpkg.StartPlain(cfg.Server.GRPCAddr, handler, psk); err != nil {
			log.Fatalf("gRPC error: %v", err)
		}
	}()

	// 14. Auth
	authHandler := api.NewAuthHandler(cfg.Auth.Username, cfg.Auth.Password, cfg.Auth.JWTSecret)

	// 15. Database (RDS) - reads from CloudStore
	dbHandler := api.NewDatabaseHandler(cloudStore, vmStore)

	// 16. Billing - reads from CloudStore + CredentialStore
	billingHandler := api.NewBillingHandler(cloudStore, credentialStore, cfg.Aliyun)

	// 17. Groups
	groupHandler := api.NewGroupHandler(groupStore, serverStore)

	// 18. New handlers
	credentialHandler := api.NewCredentialHandler(credentialStore)
	cloudHandler := api.NewCloudHandler(cloudManager, cloudStore, credentialStore)
	managedServerHandler := api.NewManagedServerHandler(managedServerStore, dep, credentialStore)

	// 19. HTTP API
	router := api.SetupRouter(api.RouterDeps{
		ServerStore:          serverStore,
		GroupStore:           groupStore,
		Hub:                  hub,
		MetricsProvider:      metricsProvider,
		StaticDir:            cfg.Server.StaticDir,
		ProbeHandler:         probeHandler,
		AssetHandler:         assetHandler,
		AuthHandler:          authHandler,
		DatabaseHandler:      dbHandler,
		BillingHandler:       billingHandler,
		AlertHandler:         alertHandler,
		GroupHandler:         groupHandler,
		CredentialHandler:    credentialHandler,
		CloudHandler:         cloudHandler,
		ManagedServerHandler: managedServerHandler,
	})
	log.Printf("HTTP server on %s, gRPC on %s", cfg.Server.HTTPAddr, cfg.Server.GRPCAddr)
	if err := router.Run(cfg.Server.HTTPAddr); err != nil {
		log.Fatalf("HTTP error: %v", err)
	}
}
