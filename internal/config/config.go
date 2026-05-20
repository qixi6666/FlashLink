package config

import (
	"os"
	"strings"
)

type Service struct {
	Name string
	Addr string
	Env  string
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

func getenv(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
