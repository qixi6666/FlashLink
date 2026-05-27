# FlashLink

FlashLink 是一个 Go 实现的高并发短链接服务，覆盖短链生成、重定向、多级缓存、防穿透、访问统计、异步批量写入和定时清理。

## 功能

- `POST /api/links` 创建短链接
- `GET /:code` 302 重定向
- `GET /api/links/:code/stats` 查询 PV、UV、今日访问量和 Referer 来源
- `LocalCache + Redis + MySQL` 多级读取
- Gin 网关可通过 gRPC 调用内部 `linksvc`、`redirectsvc`、`statsvc`
- etcd 服务注册发现
- `singleflight` 合并热点短码回源
- Redis Set 过滤不存在短码，降低缓存穿透
- 环形缓冲区 + 对象池 + worker 批量写入短链
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

再运行 gateway：

```bash
export MYSQL_DSN='flashlink:flashlink@tcp(127.0.0.1:3306)/flashlink?charset=utf8mb4&parseTime=True&loc=Local'
export REDIS_ADDR='127.0.0.1:6379'
export SHORT_LINK_DOMAIN='http://127.0.0.1:8080'
go run ./cmd/gateway
```

如果要启用 gRPC 内部服务链路，先分别在不同终端启动服务：

```bash
export ETCD_ENDPOINTS='127.0.0.1:2379'
export MYSQL_DSN='flashlink:flashlink@tcp(127.0.0.1:3306)/flashlink?charset=utf8mb4&parseTime=True&loc=Local'
export REDIS_ADDR='127.0.0.1:6379'

go run ./cmd/linksvc
go run ./cmd/redirectsvc
go run ./cmd/statsvc

export GATEWAY_USE_GRPC=true
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

重定向压测：

```bash
REQUESTS=10000 CONCURRENCY=100 bash scripts/bench_redirect.sh
```

写入压测：

```bash
REQUESTS=10000 CONCURRENCY=100 bash scripts/bench_create.sh
```

无效短码请求压测：

```bash
REQUESTS=10000 CONCURRENCY=100 bash scripts/bench_invalid.sh
```

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
