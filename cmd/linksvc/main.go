package main

import (
	"context"
	"log"
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/jd/flashlink/internal/app/linkapp"
	"github.com/jd/flashlink/internal/config"
	"github.com/jd/flashlink/internal/domain/link"
	"github.com/jd/flashlink/internal/infrastructure/cache"
	infraetcd "github.com/jd/flashlink/internal/infrastructure/etcd"
	"github.com/jd/flashlink/internal/infrastructure/filter"
	"github.com/jd/flashlink/internal/infrastructure/mysql"
	infraredis "github.com/jd/flashlink/internal/infrastructure/redis"
	"github.com/jd/flashlink/internal/interfaces/grpcapi"
)

func main() {
	cfg := config.LoadService("linksvc", ":8081")
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

	shortRepo := mysql.NewShortLinkRepository(db)
	redisFilter := filter.NewRedisSet(redisClient)
	if err := redisFilter.Rebuild(ctx, shortRepo, 1000); err != nil {
		log.Printf("rebuild redis filter failed: %v", err)
	}

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
		RedisCache: cache.NewRedis(redisClient),
		Filter:     redisFilter,
		Domain:     config.LoadShortLink().Domain,
	})

	registry, err := infraetcd.Open(config.LoadEtcd())
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = registry.Close()
	}()
	advertiseAddr := config.LoadAdvertiseAddr("linksvc", cfg.Addr)
	if err := registry.Register(ctx, grpcapi.ServiceLink, advertiseAddr, config.LoadEtcd().LeaseTTL); err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatal(err)
	}

	server := grpcapi.NewServer()
	grpcapi.RegisterLinkService(server, linkService)

	go func() {
		<-ctx.Done()
		asyncWriter.Wait()
	}()

	log.Printf("starting %s grpc on %s advertise=%s", cfg.Name, cfg.Addr, advertiseAddr)
	if err := grpcapi.Serve(ctx, server, listener); err != nil {
		log.Fatal(err)
	}
}
