package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jd/flashlink/internal/config"
	infraetcd "github.com/jd/flashlink/internal/infrastructure/etcd"
	"github.com/jd/flashlink/internal/interfaces/grpcapi"
	"github.com/jd/flashlink/internal/interfaces/httpapi"
	"google.golang.org/grpc/resolver"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.LoadService("gateway", ":8080")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runGateway(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}

func runGateway(ctx context.Context, cfg config.Service) error {
	etcdCfg := config.LoadEtcd()
	registry, err := infraetcd.Open(etcdCfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = registry.Close()
	}()
	resolver.Register(infraetcd.NewResolverBuilder(registry))

	linkTarget := infraetcd.ResolverTarget(grpcapi.ServiceLink)
	redirectTarget := infraetcd.ResolverTarget(grpcapi.ServiceRedirect)
	statsTarget := infraetcd.ResolverTarget(grpcapi.ServiceStats)

	client, err := grpcapi.NewGatewayClient(ctx, linkTarget, redirectTarget, statsTarget)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewRouter(httpapi.RouterOptions{Links: client, Stats: client, Recorder: client}),
		ReadHeaderTimeout: 3 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("gateway shutdown failed: %v", err)
		}
	}()

	log.Printf(
		"starting %s on %s via grpc linksvc=%s redirectsvc=%s statsvc=%s",
		cfg.Name,
		cfg.Addr,
		linkTarget,
		redirectTarget,
		statsTarget,
	)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
