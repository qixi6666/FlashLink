package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/jd/flashlink/internal/app/health"
	"github.com/jd/flashlink/internal/config"
)

func main() {
	cfg := config.LoadService("statsvc", ":8083")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("starting %s on %s", cfg.Name, cfg.Addr)
	if err := health.Run(ctx, cfg.Name, cfg.Addr); err != nil {
		log.Fatal(err)
	}
}
