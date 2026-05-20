package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	Name string
	Addr string
	Env  string
}

type MySQL struct {
	DSN string
}

type Redis struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type ShortLink struct {
	Domain string
}

type Cleanup struct {
	Enabled        bool
	Interval       time.Duration
	VisitRetention time.Duration
	StatRetention  time.Duration
	BatchSize      int
}

func LoadService(name string, defaultAddr string) Service {
	envKey := strings.ToUpper(name) + "_ADDR"
	addr := getenv(envKey, "")
	if addr == "" {
		addr = getenv("APP_ADDR", defaultAddr)
	}

	return Service{
		Name: name,
		Addr: addr,
		Env:  getenv("APP_ENV", "local"),
	}
}

func LoadMySQL() MySQL {
	return MySQL{
		DSN: getenv("MYSQL_DSN", ""),
	}
}

func LoadRedis() Redis {
	return Redis{
		Addr:         getenv("REDIS_ADDR", "127.0.0.1:6379"),
		Password:     getenv("REDIS_PASSWORD", ""),
		DB:           getenvInt("REDIS_DB", 0),
		PoolSize:     getenvInt("REDIS_POOL_SIZE", 20),
		DialTimeout:  getenvDuration("REDIS_DIAL_TIMEOUT", 3*time.Second),
		ReadTimeout:  getenvDuration("REDIS_READ_TIMEOUT", time.Second),
		WriteTimeout: getenvDuration("REDIS_WRITE_TIMEOUT", time.Second),
	}
}

func LoadShortLink() ShortLink {
	return ShortLink{
		Domain: getenv("SHORT_LINK_DOMAIN", "http://127.0.0.1:8080"),
	}
}

func LoadCleanup() Cleanup {
	return Cleanup{
		Enabled:        getenvBool("CLEANUP_ENABLED", true),
		Interval:       getenvDuration("CLEANUP_INTERVAL", 24*time.Hour),
		VisitRetention: getenvDuration("CLEANUP_VISIT_RETENTION", 30*24*time.Hour),
		StatRetention:  getenvDuration("CLEANUP_STAT_RETENTION", 180*24*time.Hour),
		BatchSize:      getenvInt("CLEANUP_BATCH_SIZE", 1000),
	}
}

func getenv(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
