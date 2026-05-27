package etcd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jd/flashlink/internal/config"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const servicePrefix = "/flashlink/services"

type Registry struct {
	client *clientv3.Client
}

func Open(cfg config.Etcd) (*Registry, error) {
	if len(cfg.Endpoints) == 0 {
		return nil, fmt.Errorf("etcd endpoints are required")
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 3 * time.Second
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
	})
	if err != nil {
		return nil, err
	}

	return &Registry{client: client}, nil
}

func (r *Registry) Close() error {
	return r.client.Close()
}

func (r *Registry) Register(ctx context.Context, serviceName string, addr string, ttl int64) error {
	if ttl <= 0 {
		ttl = 10
	}

	lease, err := r.client.Grant(ctx, ttl)
	if err != nil {
		return err
	}

	key := serviceKey(serviceName, addr)
	if _, err := r.client.Put(ctx, key, addr, clientv3.WithLease(lease.ID)); err != nil {
		return err
	}

	keepAlive, err := r.client.KeepAlive(ctx, lease.ID)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case _, ok := <-keepAlive:
				if !ok {
					return
				}
			case <-ctx.Done():
				revokeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, _ = r.client.Revoke(revokeCtx, lease.ID)
				cancel()
				return
			}
		}
	}()

	return nil
}

func (r *Registry) Discover(ctx context.Context, serviceName string) (string, error) {
	resp, err := r.client.Get(ctx, servicePrefix+"/"+serviceName+"/", clientv3.WithPrefix())
	if err != nil {
		return "", err
	}
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("service %q not found in etcd", serviceName)
	}
	return string(resp.Kvs[0].Value), nil
}

func serviceKey(serviceName string, addr string) string {
	addr = strings.ReplaceAll(addr, "/", "_")
	return fmt.Sprintf("%s/%s/%s", servicePrefix, serviceName, addr)
}
