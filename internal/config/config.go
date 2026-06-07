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

type Etcd struct {
	Endpoints   []string
	DialTimeout time.Duration
	LeaseTTL    int64
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

type BloomFilter struct {
	Key       string
	Capacity  uint64
	ErrorRate float64
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

func LoadEtcd() Etcd {
	raw := getenv("ETCD_ENDPOINTS", "")
	var endpoints []string
	for _, endpoint := range strings.Split(raw, ",") {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint != "" {
			endpoints = append(endpoints, endpoint)
		}
	}

	return Etcd{
		Endpoints:   endpoints,
		DialTimeout: getenvDuration("ETCD_DIAL_TIMEOUT", 3*time.Second),
		LeaseTTL:    int64(getenvInt("ETCD_LEASE_TTL", 10)),
	}
}

func LoadShortLink() ShortLink {
	return ShortLink{
		Domain: getenv("SHORT_LINK_DOMAIN", "http://127.0.0.1:8080"),
	}
}

func LoadSnowflakeNodeID(serviceName string, fallback int64) int64 {
	envKey := strings.ToUpper(serviceName) + "_NODE_ID"
	return int64(getenvInt(envKey, int(fallback)))
}

func LoadAdvertiseAddr(serviceName string, listenAddr string) string {
	envKey := strings.ToUpper(serviceName) + "_ADVERTISE_ADDR"
	if value := getenv(envKey, ""); value != "" {
		return value
	}
	return normalizeAdvertiseAddr(listenAddr)
}

func normalizeAdvertiseAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
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

func LoadBloomFilter() BloomFilter {
	capacity := getenvInt("BLOOM_FILTER_CAPACITY", 10_000_000)
	if capacity <= 0 {
		capacity = 10_000_000
	}

	errorRate := getenvFloat("BLOOM_FILTER_ERROR_RATE", 0.01)
	if errorRate <= 0 || errorRate >= 1 {
		errorRate = 0.01
	}

	return BloomFilter{
		Key:       getenv("BLOOM_FILTER_KEY", "flashlink:filter:codes"),
		Capacity:  uint64(capacity),
		ErrorRate: errorRate,
	}
}

func LoadGatewayUseGRPC() bool {
	return getenvBool("GATEWAY_USE_GRPC", false)
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

func getenvFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
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
