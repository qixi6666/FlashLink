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
	"github.com/jd/flashlink/internal/infrastructure/filter"
	"github.com/jd/flashlink/internal/infrastructure/mysql"
	infraredis "github.com/jd/flashlink/internal/infrastructure/redis"
	"github.com/jd/flashlink/internal/interfaces/httpapi"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.LoadService("gateway", ":8080")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	ids, err := link.NewSnowflake(1)
	if err != nil {
		log.Fatal(err)
	}

	shortLinkCfg := config.LoadShortLink()
	shortRepo := mysql.NewShortLinkRepository(db)
	visitRepo := mysql.NewVisitRepository(db)
	redisFilter := filter.NewRedisSet(redisClient)
	if err := redisFilter.Rebuild(ctx, shortRepo, 1000); err != nil {
		log.Printf("rebuild redis filter failed: %v", err)
	}
	redisCache := cache.NewRedis(redisClient)
	asyncWriter := linkapp.NewAsyncShortLinkWriter(ctx, linkapp.AsyncWriterOptions{
		Repository:    shortRepo,
		BatchWriter:   shortRepo,
		QueueSize:     8192,
		BatchSize:     256,
		Workers:       4,
		FlushInterval: 10 * time.Millisecond,
	})

	linkService := linkapp.New(linkapp.Options{
		Repository: asyncWriter,
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
		asyncWriter.Wait()
	}()

	log.Printf("starting %s on %s", cfg.Name, cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
