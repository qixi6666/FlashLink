package redis

import (
	"github.com/jd/flashlink/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

func Open(cfg config.Redis) *goredis.Client {
	return goredis.NewClient(&goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})
}
