//go:build linux

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mantisops/agent/internal/config"
	"mantisops/agent/internal/reporter"
)

func main() {
	cfgPath := flag.String("config", "/etc/mantisops/agent.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	r := reporter.New(cfg)
	if err := r.Connect(); err != nil {
		log.Fatalf("connect server: %v", err)
	}
	defer r.Close()

	if err := r.Register(); err != nil {
		log.Printf("register warning: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down...")
		cancel()
	}()

	log.Printf("agent started, reporting to %s every %ds (docker=%v, gpu=%v)", cfg.Server.Address, cfg.Collect.Interval, cfg.Collect.Docker, cfg.Collect.GPU)
	r.RunLoop(ctx)
}
