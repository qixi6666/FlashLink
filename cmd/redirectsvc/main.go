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
	"github.com/jd/flashlink/internal/infrastructure/cache"
	infraetcd "github.com/jd/flashlink/internal/infrastructure/etcd"
	"github.com/jd/flashlink/internal/infrastructure/filter"
	"github.com/jd/flashlink/internal/infrastructure/mysql"
	infraredis "github.com/jd/flashlink/internal/infrastructure/redis"
	"github.com/jd/flashlink/internal/interfaces/grpcapi"
)

func main() {
	cfg := config.LoadService("redirectsvc", ":8082")
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

	shortRepo := mysql.NewShortLinkRepository(db)
	bloomCfg := config.LoadBloomFilter()
	redisFilter := filter.NewRedisBloom(redisClient, filter.RedisBloomOptions{
		Key:       bloomCfg.Key,
		Capacity:  bloomCfg.Capacity,
		ErrorRate: bloomCfg.ErrorRate,
	})
	if err := redisFilter.Rebuild(ctx, shortRepo, 1000); err != nil {
		log.Printf("rebuild redis filter failed: %v", err)
	}

	linkService := linkapp.New(linkapp.Options{
		Repository: shortRepo,
		LocalCache: cache.NewLocalWithMaxEntries(10000),
		RedisCache: cache.NewRedis(redisClient),
		Filter:     redisFilter,
		Domain:     config.LoadShortLink().Domain,
	})

	etcdCfg := config.LoadEtcd()
	registry, err := infraetcd.Open(etcdCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = registry.Close()
	}()
	advertiseAddr := config.LoadAdvertiseAddr("redirectsvc", cfg.Addr)
	if err := registry.Register(ctx, grpcapi.ServiceRedirect, advertiseAddr, etcdCfg.LeaseTTL); err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatal(err)
	}

	server := grpcapi.NewServer()
	grpcapi.RegisterRedirectService(server, linkService)

	log.Printf("starting %s grpc on %s advertise=%s", cfg.Name, cfg.Addr, advertiseAddr)
	if err := grpcapi.Serve(ctx, server, listener); err != nil {
		log.Fatal(err)
	}
}
