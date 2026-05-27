package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type config struct {
	baseURL     string
	codesFile   string
	requests    int
	concurrency int
	expected    int
	mode        string
	timeout     time.Duration
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := parseFlags()
	if cfg.requests <= 0 {
		return fmt.Errorf("requests must be positive")
	}
	if cfg.concurrency <= 0 {
		return fmt.Errorf("concurrency must be positive")
	}
	if cfg.mode != "random" && cfg.mode != "sequential" {
		return fmt.Errorf("mode must be random or sequential")
	}

	codes, err := readCodes(cfg.codesFile)
	if err != nil {
		return err
	}
	if len(codes) == 0 {
		return fmt.Errorf("no codes found in %s", cfg.codesFile)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return runRead(ctx, cfg, codes)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.baseURL, "base", "http://127.0.0.1:8080", "base service URL")
	flag.StringVar(&cfg.codesFile, "codes", "tmp/flashlink_codes.txt", "file containing one short code per line")
	flag.IntVar(&cfg.requests, "n", 10000, "number of requests")
	flag.IntVar(&cfg.concurrency, "c", 100, "concurrent workers")
	flag.IntVar(&cfg.expected, "expected", http.StatusFound, "expected HTTP status code")
	flag.StringVar(&cfg.mode, "mode", "random", "code selection mode: random or sequential")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Second, "per-request timeout")
	flag.Parse()
	cfg.baseURL = strings.TrimRight(cfg.baseURL, "/")
	return cfg
}

func readCodes(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var codes []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		code := strings.TrimSpace(scanner.Text())
		if code != "" {
			codes = append(codes, code)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return codes, nil
}

func runRead(ctx context.Context, cfg config, codes []string) error {
	client := &http.Client{
		Timeout: cfg.timeout,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.concurrency * 2,
			MaxIdleConnsPerHost: cfg.concurrency * 2,
			MaxConnsPerHost:     cfg.concurrency * 2,
			IdleConnTimeout:     30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	statuses := make([]int, cfg.requests)
	latencies := make([]time.Duration, cfg.requests)

	var next atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	for worker := 0; worker < cfg.concurrency; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(worker)))
			for {
				index := int(next.Add(1)) - 1
				if index >= cfg.requests {
					return
				}
				code := selectCode(cfg.mode, codes, index, rng)
				statuses[index], latencies[index] = doGet(ctx, client, cfg.baseURL+"/"+code)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	printReport(cfg, len(codes), statuses, latencies, elapsed)
	return nil
}

func selectCode(mode string, codes []string, index int, rng *rand.Rand) string {
	if mode == "sequential" {
		return codes[index%len(codes)]
	}
	return codes[rng.Intn(len(codes))]
}

func doGet(ctx context.Context, client *http.Client, url string) (int, time.Duration) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	return resp.StatusCode, elapsed
}

func printReport(cfg config, codeCount int, statuses []int, latencies []time.Duration, elapsed time.Duration) {
	statusCounts := make(map[int]int)
	success := 0
	for _, status := range statuses {
		statusCounts[status]++
		if status == cfg.expected {
			success++
		}
	}

	sorted := append([]time.Duration(nil), latencies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	fmt.Printf("mode=%s codes=%d requests=%d concurrency=%d expected_status=%d\n", cfg.mode, codeCount, cfg.requests, cfg.concurrency, cfg.expected)
	fmt.Printf("total=%.4fs qps=%.2f success=%d failed=%d success_rate=%.2f%%\n",
		elapsed.Seconds(),
		float64(cfg.requests)/elapsed.Seconds(),
		success,
		cfg.requests-success,
		float64(success)*100/float64(cfg.requests),
	)
	fmt.Printf("latency_avg=%s p50=%s p95=%s p99=%s max=%s\n",
		avgLatency(sorted),
		percentile(sorted, 0.50),
		percentile(sorted, 0.95),
		percentile(sorted, 0.99),
		sorted[len(sorted)-1],
	)
	fmt.Println("status_codes:")
	for _, status := range sortedStatusCodes(statusCounts) {
		fmt.Printf("  %d=%d\n", status, statusCounts[status])
	}
}

func avgLatency(values []time.Duration) time.Duration {
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
	index := int(float64(len(values)-1) * p)
	return values[index]
}

func sortedStatusCodes(counts map[int]int) []int {
	codes := make([]int, 0, len(counts))
	for code := range counts {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}
