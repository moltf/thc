package scanner

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moltf/thc/internal/api"
	"github.com/moltf/thc/internal/output"
)

type Config struct {
	Workers    int
	Quiet      bool
	NoProgress bool
}

type Stats struct {
	Targets    int
	Processed  int
	Domains    int
	Duplicates int
	Requests   int
	Errors     int
	Duration   time.Duration
}

type Scanner struct {
	client *api.Client
	writer *output.Writer
	cfg    Config
}

func New(
	client *api.Client,
	writer *output.Writer,
	cfg Config,
) *Scanner {
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}

	return &Scanner{
		client: client,
		writer: writer,
		cfg:    cfg,
	}
}

func (s *Scanner) Run(
	ctx context.Context,
	targets []string,
) Stats {
	start := time.Now()

	stats := Stats{
		Targets: len(targets),
	}

	jobs := make(chan string)

	var wg sync.WaitGroup

	var domainCount atomic.Int64
	var duplicateCount atomic.Int64
	var requestCount atomic.Int64
	var errorCount atomic.Int64
	var processed atomic.Int64

	seen := make(map[string]struct{})
	var seenMu sync.Mutex

	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for target := range jobs {
				if ctx.Err() != nil {
					return
				}

				result, err := s.client.Lookup(
					ctx,
					target,
				)

				if err != nil {
					errorCount.Add(1)

					if !s.cfg.Quiet {
						fmt.Fprintf(
							os.Stderr,
							"[-] %s: %v\n",
							target,
							err,
						)
					}

					continue
				}

				requestCount.Add(
					int64(result.Requests),
				)

				for _, domain := range result.Domains {
					seenMu.Lock()

					_, exists := seen[domain]

					if !exists {
						seen[domain] = struct{}{}
					}

					seenMu.Unlock()

					if exists {
						duplicateCount.Add(1)
						continue
					}

					err := s.writer.Write(
						output.Record{
							Target: target,
							Domain: domain,
						},
					)

					if err != nil {
						errorCount.Add(1)

						if !s.cfg.Quiet {
							fmt.Fprintf(
								os.Stderr,
								"[-] output: %v\n",
								err,
							)
						}

						continue
					}

					domainCount.Add(1)

					if !s.cfg.Quiet &&
						s.cfg.NoProgress {
						fmt.Println(domain)
					}
				}

				processed.Add(1)
			}
		}()
	}

	go func() {
		defer close(jobs)

		for _, target := range targets {
			select {
			case <-ctx.Done():
				return

			case jobs <- target:
			}
		}
	}()

	if !s.cfg.Quiet &&
		!s.cfg.NoProgress {

		ticker := time.NewTicker(
			500 * time.Millisecond,
		)

		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				fmt.Printf(
					"\r[*] %d/%d targets | %d domains | %d requests | %d errors",
					processed.Load(),
					len(targets),
					domainCount.Load(),
					requestCount.Load(),
					errorCount.Load(),
				)

			case <-ctx.Done():
				wg.Wait()
				fmt.Println()

				stats.Processed = int(
					processed.Load(),
				)

				stats.Domains = int(
					domainCount.Load(),
				)

				stats.Duplicates = int(
					duplicateCount.Load(),
				)

				stats.Requests = int(
					requestCount.Load(),
				)

				stats.Errors = int(
					errorCount.Load(),
				)

				stats.Duration = time.Since(start)

				return stats

			default:
				if processed.Load() >= int64(len(targets)) {
					wg.Wait()
					fmt.Println()

					stats.Processed = int(
						processed.Load(),
					)

					stats.Domains = int(
						domainCount.Load(),
					)

					stats.Duplicates = int(
						duplicateCount.Load(),
					)

					stats.Requests = int(
						requestCount.Load(),
					)

					stats.Errors = int(
						errorCount.Load(),
					)

					stats.Duration = time.Since(start)

					return stats
				}
			}
		}
	}

	wg.Wait()

	stats.Processed = int(
		processed.Load(),
	)

	stats.Domains = int(
		domainCount.Load(),
	)

	stats.Duplicates = int(
		duplicateCount.Load(),
	)

	stats.Requests = int(
		requestCount.Load(),
	)

	stats.Errors = int(
		errorCount.Load(),
	)

	stats.Duration = time.Since(start)

	return stats
}
