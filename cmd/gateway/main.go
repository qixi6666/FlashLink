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

	"github.com/jd/flashlink/internal/app/cleanupapp"
	"github.com/jd/flashlink/internal/app/linkapp"
	"github.com/jd/flashlink/internal/app/statapp"
	"github.com/jd/flashlink/internal/config"
	"github.com/jd/flashlink/internal/domain/link"
	"github.com/jd/flashlink/internal/infrastructure/cache"
	infraetcd "github.com/jd/flashlink/internal/infrastructure/etcd"
	"github.com/jd/flashlink/internal/infrastructure/filter"
	"github.com/jd/flashlink/internal/infrastructure/mysql"
	infraredis "github.com/jd/flashlink/internal/infrastructure/redis"
	"github.com/jd/flashlink/internal/interfaces/grpcapi"
	"github.com/jd/flashlink/internal/interfaces/httpapi"
	"google.golang.org/grpc/resolver"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.LoadService("gateway", ":8080")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if config.LoadGatewayUseGRPC() {
		if err := runGRPCGateway(ctx, cfg); err != nil {
			log.Fatal(err)
		}
		return
	}

	mysqlCfg := config.LoadMySQL()
	if mysqlCfg.DSN == "" {
		log.Fatal("MYSQL_DSN is required")
	}

	db, err := mysql.Open(mysql.Config{
		DSN:             mysqlCfg.DSN,
		MaxIdleConns:    10,
		MaxOpenConns:    50,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		log.Fatal(err)
	}

	redisClient := infraredis.Open(config.LoadRedis())
	defer func() {
		_ = redisClient.Close()
	}()

	ids, err := link.NewSnowflake(config.LoadSnowflakeNodeID("gateway", 1))
	if err != nil {
		log.Fatal(err)
	}

	shortLinkCfg := config.LoadShortLink()
	bloomCfg := config.LoadBloomFilter()
	shortRepo := mysql.NewShortLinkRepository(db)
	visitRepo := mysql.NewVisitRepository(db)
	redisFilter := filter.NewRedisBloom(redisClient, filter.RedisBloomOptions{
		Key:       bloomCfg.Key,
		Capacity:  bloomCfg.Capacity,
		ErrorRate: bloomCfg.ErrorRate,
	})
	if err := redisFilter.Rebuild(ctx, shortRepo, 1000); err != nil {
		log.Printf("rebuild redis filter failed: %v", err)
	}
	redisCache := cache.NewRedis(redisClient)

	linkService := linkapp.New(linkapp.Options{
		Repository: shortRepo,
		IDs:        ids,
		LocalCache: cache.NewLocalWithMaxEntries(10000),
		RedisCache: redisCache,
		Filter:     redisFilter,
		Domain:     shortLinkCfg.Domain,
	})
	statsService := statapp.NewService(visitRepo)
	recorder := statapp.NewRecorder(statapp.RecorderOptions{
		Repository: visitRepo,
		IDs:        ids,
	})
	recorder.Start(ctx)

	cleanupCfg := config.LoadCleanup()
	if cleanupCfg.Enabled {
		cleanupService := cleanupapp.New(cleanupapp.Options{
			Links:          shortRepo,
			Visits:         visitRepo,
			Cache:          redisCache,
			Filter:         redisFilter,
			BatchSize:      cleanupCfg.BatchSize,
			VisitRetention: cleanupCfg.VisitRetention,
			StatRetention:  cleanupCfg.StatRetention,
		})
		cleanupapp.NewScheduler(cleanupService, cleanupCfg.Interval, log.Default()).Start(ctx)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewRouter(httpapi.RouterOptions{Links: linkService, Stats: statsService, Recorder: recorder}),
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

	log.Printf("starting %s on %s", cfg.Name, cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func runGRPCGateway(ctx context.Context, cfg config.Service) error {
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
