package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type config struct {
	baseURL        string
	levels         []int
	duration       time.Duration
	writeRatio     float64
	seedCodes      int
	scenario       string
	hotKeys        int
	timeout        time.Duration
	minSuccessRate float64
}

const (
	scenarioMixed  = "mixed"
	scenarioHotKey = "hot-key"
)

type createResponse struct {
	Code string `json:"code"`
}

type codePool struct {
	mu    sync.RWMutex
	codes []string
}

func (p *codePool) add(code string) {
	if code == "" {
		return
	}
	p.mu.Lock()
	p.codes = append(p.codes, code)
	p.mu.Unlock()
}

func (p *codePool) random(rng *rand.Rand) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.codes) == 0 {
		return "", false
	}
	return p.codes[rng.Intn(len(p.codes))], true
}

func (p *codePool) randomFirst(rng *rand.Rand, count int) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.codes) == 0 || count <= 0 {
		return "", false
	}
	if count > len(p.codes) {
		count = len(p.codes)
	}
	return p.codes[rng.Intn(count)], true
}

func (p *codePool) len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.codes)
}

type opStats struct {
	total     int
	success   int
	statuses  map[int]int
	latencies []time.Duration
}

func newOpStats() opStats {
	return opStats{statuses: make(map[int]int)}
}

func (s *opStats) record(status int, latency time.Duration, ok bool) {
	s.total++
	if ok {
		s.success++
	}
	s.statuses[status]++
	s.latencies = append(s.latencies, latency)
}

func (s *opStats) merge(other opStats) {
	s.total += other.total
	s.success += other.success
	for status, count := range other.statuses {
		s.statuses[status] += count
	}
	s.latencies = append(s.latencies, other.latencies...)
}

type levelResult struct {
	concurrency int
	elapsed     time.Duration
	write       opStats
	read        opStats
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := parseFlags()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := newHTTPClient(cfg)
	pool := &codePool{}
	seedCodes := cfg.seedCodes
	if cfg.scenario == scenarioHotKey && seedCodes < cfg.hotKeys {
		seedCodes = cfg.hotKeys
	}
	if seedCodes > 0 {
		if err := seed(ctx, client, cfg, pool, seedCodes); err != nil {
			return err
		}
	}

	var best *levelResult
	for _, level := range cfg.levels {
		result := runLevel(ctx, client, cfg, pool, level)
		printLevel(cfg, result, pool.len())
		if stable(result, cfg.minSuccessRate) && (best == nil || result.totalQPS() > best.totalQPS()) {
			copy := result
			best = &copy
		}
		if ctx.Err() != nil {
			break
		}
	}

	if best == nil {
		fmt.Printf("best_stable=none min_success_rate=%.2f%%\n", cfg.minSuccessRate)
		return nil
	}

	fmt.Printf(
		"best_stable scenario=%s concurrency=%d total_qps=%.2f write_qps=%.2f read_qps=%.2f success_rate=%.2f%%\n",
		cfg.scenario,
		best.concurrency,
		best.totalQPS(),
		best.writeQPS(),
		best.readQPS(),
		best.successRate(),
	)
	return nil
}

func parseFlags() (config, error) {
	var levels string
	var cfg config
	flag.StringVar(&cfg.baseURL, "base", "http://127.0.0.1:8080", "base service URL")
	flag.StringVar(&levels, "levels", "50,100,200,300,500", "comma-separated concurrency levels")
	flag.DurationVar(&cfg.duration, "duration", 20*time.Second, "duration for each concurrency level")
	flag.Float64Var(&cfg.writeRatio, "write-ratio", 0.30, "fraction of requests that create short links")
	flag.IntVar(&cfg.seedCodes, "seed-codes", 200, "short links to create before measured traffic")
	flag.StringVar(&cfg.scenario, "scenario", scenarioMixed, "load scenario: mixed or hot-key")
	flag.IntVar(&cfg.hotKeys, "hot-keys", 1, "number of fixed hot keys used by the hot-key scenario")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Second, "per-request timeout")
	flag.Float64Var(&cfg.minSuccessRate, "min-success", 99.0, "minimum success rate for a stable level")
	flag.Parse()

	cfg.baseURL = strings.TrimRight(cfg.baseURL, "/")
	parsedLevels, err := parseLevels(levels)
	if err != nil {
		return config{}, err
	}
	cfg.levels = parsedLevels
	if cfg.duration <= 0 {
		return config{}, fmt.Errorf("duration must be positive")
	}
	if cfg.writeRatio < 0 || cfg.writeRatio > 1 {
		return config{}, fmt.Errorf("write-ratio must be between 0 and 1")
	}
	if cfg.seedCodes < 0 {
		return config{}, fmt.Errorf("seed-codes must be zero or positive")
	}
	if cfg.scenario != scenarioMixed && cfg.scenario != scenarioHotKey {
		return config{}, fmt.Errorf("scenario must be %s or %s", scenarioMixed, scenarioHotKey)
	}
	if cfg.hotKeys <= 0 {
		return config{}, fmt.Errorf("hot-keys must be positive")
	}
	if cfg.timeout <= 0 {
		return config{}, fmt.Errorf("timeout must be positive")
	}
	if cfg.minSuccessRate < 0 || cfg.minSuccessRate > 100 {
		return config{}, fmt.Errorf("min-success must be between 0 and 100")
	}
	return cfg, nil
}

func parseLevels(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	levels := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		level, err := strconv.Atoi(part)
		if err != nil || level <= 0 {
			return nil, fmt.Errorf("invalid concurrency level %q", part)
		}
		levels = append(levels, level)
	}
	if len(levels) == 0 {
		return nil, fmt.Errorf("levels must contain at least one positive integer")
	}
	return levels, nil
}

func newHTTPClient(cfg config) *http.Client {
	return &http.Client{
		Timeout: cfg.timeout,
		Transport: &http.Transport{
			MaxIdleConns:        4096,
			MaxIdleConnsPerHost: 4096,
			MaxConnsPerHost:     4096,
			IdleConnTimeout:     30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func seed(ctx context.Context, client *http.Client, cfg config, pool *codePool, count int) error {
	fmt.Printf("seeding_codes=%d base=%s\n", count, cfg.baseURL)
	var ok int
	for i := 0; i < count; i++ {
		status, _, code := doCreate(ctx, client, cfg.baseURL, uint64(i+1))
		if status == http.StatusCreated && code != "" {
			pool.add(code)
			ok++
		}
	}
	if ok == 0 {
		return fmt.Errorf("could not create any seed short links")
	}
	fmt.Printf("seeded_codes=%d\n", ok)
	return nil
}

func runLevel(parent context.Context, client *http.Client, cfg config, pool *codePool, concurrency int) levelResult {
	ctx, cancel := context.WithTimeout(parent, cfg.duration)
	defer cancel()

	var seq atomic.Uint64
	results := make(chan levelResult, concurrency)
	start := make(chan struct{})
	var wg sync.WaitGroup
	started := time.Now()

	for worker := 0; worker < concurrency; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(worker)))
			local := levelResult{
				concurrency: concurrency,
				write:       newOpStats(),
				read:        newOpStats(),
			}

			for ctx.Err() == nil {
				if rng.Float64() < cfg.writeRatio {
					status, latency, code := doCreate(ctx, client, cfg.baseURL, seq.Add(1))
					if stoppedByLevelTimeout(ctx, status) {
						break
					}
					ok := status == http.StatusCreated && code != ""
					local.write.record(status, latency, ok)
					if ok {
						pool.add(code)
					}
					continue
				}

				code, ok := selectReadCode(cfg, pool, rng)
				if !ok {
					status, latency, code := doCreate(ctx, client, cfg.baseURL, seq.Add(1))
					if stoppedByLevelTimeout(ctx, status) {
						break
					}
					ok := status == http.StatusCreated && code != ""
					local.write.record(status, latency, ok)
					if ok {
						pool.add(code)
					}
					continue
				}
				status, latency := doRead(ctx, client, cfg.baseURL, code)
				if stoppedByLevelTimeout(ctx, status) {
					break
				}
				local.read.record(status, latency, status == http.StatusFound)
			}
			results <- local
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	merged := levelResult{
		concurrency: concurrency,
		elapsed:     time.Since(started),
		write:       newOpStats(),
		read:        newOpStats(),
	}
	for result := range results {
		merged.write.merge(result.write)
		merged.read.merge(result.read)
	}
	return merged
}

func selectReadCode(cfg config, pool *codePool, rng *rand.Rand) (string, bool) {
	if cfg.scenario == scenarioHotKey {
		return pool.randomFirst(rng, cfg.hotKeys)
	}
	return pool.random(rng)
}

func stoppedByLevelTimeout(ctx context.Context, status int) bool {
	return status == 0 && ctx.Err() != nil
}

func doCreate(ctx context.Context, client *http.Client, baseURL string, id uint64) (int, time.Duration, string) {
	body := fmt.Sprintf(`{"long_url":"https://example.com/mixed-load/%d/%d"}`, time.Now().UnixNano(), id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/links", bytes.NewBufferString(body))
	if err != nil {
		return 0, 0, ""
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, elapsed, ""
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusCreated {
		return resp.StatusCode, elapsed, ""
	}
	var created createResponse
	if err := json.Unmarshal(data, &created); err != nil {
		return resp.StatusCode, elapsed, ""
	}
	return resp.StatusCode, elapsed, created.Code
}

func doRead(ctx context.Context, client *http.Client, baseURL, code string) (int, time.Duration) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/"+code, nil)
	if err != nil {
		return 0, 0
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, elapsed
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, elapsed
}

func printLevel(cfg config, result levelResult, codeCount int) {
	fmt.Printf(
		"level scenario=%s hot_keys=%d concurrency=%d duration=%.2fs codes=%d total_qps=%.2f write_qps=%.2f read_qps=%.2f success_rate=%.2f%% writes=%d/%d reads=%d/%d\n",
		cfg.scenario,
		cfg.hotKeys,
		result.concurrency,
		result.elapsed.Seconds(),
		codeCount,
		result.totalQPS(),
		result.writeQPS(),
		result.readQPS(),
		result.successRate(),
		result.write.success,
		result.write.total,
		result.read.success,
		result.read.total,
	)
	fmt.Printf(
		"  write_latency avg=%s p50=%s p95=%s p99=%s max=%s statuses=%s\n",
		avg(result.write.latencies),
		percentile(result.write.latencies, 0.50),
		percentile(result.write.latencies, 0.95),
		percentile(result.write.latencies, 0.99),
		maxLatency(result.write.latencies),
		formatStatuses(result.write.statuses),
	)
	fmt.Printf(
		"  read_latency  avg=%s p50=%s p95=%s p99=%s max=%s statuses=%s\n",
		avg(result.read.latencies),
		percentile(result.read.latencies, 0.50),
		percentile(result.read.latencies, 0.95),
		percentile(result.read.latencies, 0.99),
		maxLatency(result.read.latencies),
		formatStatuses(result.read.statuses),
	)
}

func stable(result levelResult, minSuccessRate float64) bool {
	return result.total() > 0 && result.successRate() >= minSuccessRate
}

func (r levelResult) total() int {
	return r.write.total + r.read.total
}

func (r levelResult) success() int {
	return r.write.success + r.read.success
}

func (r levelResult) totalQPS() float64 {
	return float64(r.total()) / r.elapsed.Seconds()
}

func (r levelResult) writeQPS() float64 {
	return float64(r.write.total) / r.elapsed.Seconds()
}

func (r levelResult) readQPS() float64 {
	return float64(r.read.total) / r.elapsed.Seconds()
}

func (r levelResult) successRate() float64 {
	if r.total() == 0 {
		return 0
	}
	return float64(r.success()) * 100 / float64(r.total())
}

func avg(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	var total time.Duration
	for _, value := range values {
		total += value
	}
	return total / time.Duration(len(values))
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

func maxLatency(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, value := range values[1:] {
		if value > max {
			max = value
		}
	}
	return max
}

func formatStatuses(statuses map[int]int) string {
	if len(statuses) == 0 {
		return "{}"
	}
	codes := make([]int, 0, len(statuses))
	for code := range statuses {
		codes = append(codes, code)
	}
	sort.Ints(codes)

	parts := make([]string, 0, len(codes))
	for _, code := range codes {
		parts = append(parts, fmt.Sprintf("%d:%d", code, statuses[code]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
