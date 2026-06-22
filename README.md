# FlashLink

FlashLink 是一个 Go 实现的高并发短链接服务，覆盖短链生成、重定向、多级缓存、防穿透、访问统计、异步批量写入和定时清理。

## 功能

- `POST /api/links` 创建短链接
- `GET /:code` 302 重定向
- `GET /api/links/:code/stats` 查询 PV、UV、今日访问量和 Referer 来源
- `LocalCache + Redis + MySQL` 多级读取
- Gin 网关通过 gRPC 调用内部 `linksvc`、`redirectsvc`、`statsvc`
- etcd 服务注册发现
- `singleflight` 合并热点短码回源
- RedisBloom 布隆过滤器过滤不存在短码，降低缓存穿透
- 创建短链同步写入 MySQL，成功后更新布隆过滤器并预热缓存
- 访问日志异步批量写入
- 定时清理过期短链、访问日志、统计数据并重建过滤器
- JSON 请求日志和 `/debug/pprof`

## 快速启动

```bash
docker compose up --build
```

服务启动后：

```bash
curl http://127.0.0.1:8080/healthz
```

## 本地运行

先启动依赖：

```bash
docker compose up mysql redis etcd
```

再分别在不同终端启动内部服务和 gateway：

```bash
export ETCD_ENDPOINTS='127.0.0.1:2379'
export MYSQL_DSN='flashlink:flashlink@tcp(127.0.0.1:3306)/flashlink?charset=utf8mb4&parseTime=True&loc=Local'
export REDIS_ADDR='127.0.0.1:6379'

go run ./cmd/linksvc
go run ./cmd/redirectsvc
go run ./cmd/statsvc

go run ./cmd/gateway
```

## 接口示例

创建短链：

```bash
curl -X POST http://127.0.0.1:8080/api/links \
  -H 'Content-Type: application/json' \
  -d '{"long_url":"https://example.com/product/1"}'
```

访问短链：

```bash
curl -I http://127.0.0.1:8080/{code}
```

查询统计：

```bash
curl http://127.0.0.1:8080/api/links/{code}/stats
```

## 验证

```bash
make test
make race
make bench
```

冒烟测试：

```bash
bash scripts/smoke.sh
```

混合读写压测：

```bash
go run ./cmd/loadtest -base http://127.0.0.1:8080 -levels 50,100,200,300,500 -duration 20s -write-ratio 0.3
```

热点 key 上线压测：

```bash
go run ./cmd/loadtest -base http://127.0.0.1:8080 -scenario hot-key -hot-keys 1 -levels 100,300,500,800,1000 -duration 30s -write-ratio 0.3
```

压测工具会同时创建短链和读取短链，并输出每个并发档位的总 QPS、写 QPS、读 QPS、成功率、p50、p95、p99 和最高稳定吞吐。历史压测结果记录在 [docs/loadtest-qps.md](docs/loadtest-qps.md)。

本机基准测试参考：

```text
BenchmarkAsyncShortLinkWriterCreate-16  1681676  598.6 ns/op
```

## 可观测性

请求日志以 JSON 输出到 stdout。

pprof：

```bash
curl http://127.0.0.1:8080/debug/pprof/goroutine?debug=1
go tool pprof http://127.0.0.1:8080/debug/pprof/profile
```

## 配置

参考 [configs/app.env.example](configs/app.env.example)。
