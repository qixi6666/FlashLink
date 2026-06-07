package main

import (
	"context"
	"log"
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/jd/flashlink/internal/app/statapp"
	"github.com/jd/flashlink/internal/config"
	"github.com/jd/flashlink/internal/domain/link"
	infraetcd "github.com/jd/flashlink/internal/infrastructure/etcd"
	"github.com/jd/flashlink/internal/infrastructure/mysql"
	"github.com/jd/flashlink/internal/interfaces/grpcapi"
)

func main() {
	cfg := config.LoadService("statsvc", ":8083")
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

	ids, err := link.NewSnowflake(config.LoadSnowflakeNodeID("statsvc", 2))
	if err != nil {
		log.Fatal(err)
	}

	visitRepo := mysql.NewVisitRepository(db)
	statsService := statapp.NewService(visitRepo)
	recorder := statapp.NewRecorder(statapp.RecorderOptions{
		Repository: visitRepo,
		IDs:        ids,
	})
	recorder.Start(ctx)

	etcdCfg := config.LoadEtcd()
	registry, err := infraetcd.Open(etcdCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = registry.Close()
	}()
	advertiseAddr := config.LoadAdvertiseAddr("statsvc", cfg.Addr)
	if err := registry.Register(ctx, grpcapi.ServiceStats, advertiseAddr, etcdCfg.LeaseTTL); err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatal(err)
	}

	server := grpcapi.NewServer()
	grpcapi.RegisterStatService(server, statsService, recorder)

	log.Printf("starting %s grpc on %s advertise=%s", cfg.Name, cfg.Addr, advertiseAddr)
	if err := grpcapi.Serve(ctx, server, listener); err != nil {
		log.Fatal(err)
	}
}
