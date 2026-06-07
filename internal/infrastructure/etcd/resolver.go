package etcd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/resolver"
)

const ResolverScheme = "etcd"

func ResolverTarget(serviceName string) string {
	return ResolverScheme + ":///" + strings.Trim(serviceName, "/")
}

func NewResolverBuilder(registry *Registry) resolver.Builder {
	return &resolverBuilder{registry: registry}
}

type resolverBuilder struct {
	registry *Registry
}

func (b *resolverBuilder) Scheme() string {
	return ResolverScheme
}

func (b *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	if b.registry == nil {
		return nil, fmt.Errorf("etcd registry is required")
	}

	serviceName := strings.TrimSpace(target.Endpoint())
	if serviceName == "" {
		return nil, fmt.Errorf("etcd resolver target must include a service name")
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &watchResolver{
		ctx:         ctx,
		cancel:      cancel,
		registry:    b.registry,
		serviceName: serviceName,
		prefix:      servicePath(serviceName),
		cc:          cc,
		instances:   make(map[string]string),
	}

	revision, err := r.resolveNow()
	if err != nil {
		cancel()
		return nil, err
	}

	go r.watch(revision + 1)
	return r, nil
}

type watchResolver struct {
	ctx         context.Context
	cancel      context.CancelFunc
	registry    *Registry
	serviceName string
	prefix      string
	cc          resolver.ClientConn

	mu        sync.Mutex
	instances map[string]string
}

func (r *watchResolver) ResolveNow(resolver.ResolveNowOptions) {
	go func() {
		if _, err := r.resolveNow(); err != nil {
			r.cc.ReportError(err)
		}
	}()
}

func (r *watchResolver) Close() {
	r.cancel()
}

func (r *watchResolver) resolveNow() (int64, error) {
	resp, err := r.registry.client.Get(r.ctx, r.prefix, clientv3.WithPrefix())
	if err != nil {
		return 0, err
	}

	next := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		addr := strings.TrimSpace(string(kv.Value))
		if addr != "" {
			next[string(kv.Key)] = addr
		}
	}

	r.mu.Lock()
	r.instances = next
	r.mu.Unlock()
	r.updateState()

	return resp.Header.Revision, nil
}

func (r *watchResolver) watch(revision int64) {
	watchCh := r.registry.client.Watch(r.ctx, r.prefix, clientv3.WithPrefix(), clientv3.WithRev(revision))
	for resp := range watchCh {
		if err := resp.Err(); err != nil {
			r.cc.ReportError(err)
			continue
		}

		changed := false
		r.mu.Lock()
		for _, event := range resp.Events {
			key := string(event.Kv.Key)
			switch event.Type {
			case mvccpb.PUT:
				addr := strings.TrimSpace(string(event.Kv.Value))
				if addr == "" {
					delete(r.instances, key)
				} else {
					r.instances[key] = addr
				}
				changed = true
			case mvccpb.DELETE:
				delete(r.instances, key)
				changed = true
			}
		}
		r.mu.Unlock()

		if changed {
			r.updateState()
		}
	}
}

func (r *watchResolver) updateState() {
	addresses := r.addresses()
	if len(addresses) == 0 {
		r.cc.ReportError(fmt.Errorf("service %q has no registered instances", r.serviceName))
	}

	state := resolver.State{Addresses: addresses}
	if err := r.cc.UpdateState(state); err != nil {
		r.cc.ReportError(err)
	}
}

func (r *watchResolver) addresses() []resolver.Address {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[string]struct{}, len(r.instances))
	addrs := make([]string, 0, len(r.instances))
	for _, addr := range r.instances {
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	resolved := make([]resolver.Address, 0, len(addrs))
	for _, addr := range addrs {
		resolved = append(resolved, resolver.Address{Addr: addr})
	}
	return resolved
}
