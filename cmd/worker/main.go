package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/jd/flashlink/internal/app/cleanupapp"
	"github.com/jd/flashlink/internal/config"
	"github.com/jd/flashlink/internal/infrastructure/cache"
	"github.com/jd/flashlink/internal/infrastructure/filter"
	"github.com/jd/flashlink/internal/infrastructure/mysql"
	infraredis "github.com/jd/flashlink/internal/infrastructure/redis"
)

func main() {
	cfg := config.LoadService("worker", ":8084")
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

	cleanupCfg := config.LoadCleanup()
	bloomCfg := config.LoadBloomFilter()
	cleanupService := cleanupapp.New(cleanupapp.Options{
		Links:  mysql.NewShortLinkRepository(db),
		Visits: mysql.NewVisitRepository(db),
		Cache:  cache.NewRedis(redisClient),
		Filter: filter.NewRedisBloom(redisClient, filter.RedisBloomOptions{
			Key:       bloomCfg.Key,
			Capacity:  bloomCfg.Capacity,
			ErrorRate: bloomCfg.ErrorRate,
		}),
		BatchSize:      cleanupCfg.BatchSize,
		VisitRetention: cleanupCfg.VisitRetention,
		StatRetention:  cleanupCfg.StatRetention,
	})

	if cleanupCfg.Enabled {
		cleanupapp.NewScheduler(cleanupService, cleanupCfg.Interval, log.Default()).Start(ctx)
	}

	log.Printf("starting %s cleanup worker enabled=%t interval=%s", cfg.Name, cleanupCfg.Enabled, cleanupCfg.Interval)
	<-ctx.Done()
}
