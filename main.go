package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/moltf/thc/internal/api"
	"github.com/moltf/thc/internal/config"
	"github.com/moltf/thc/internal/output"
	"github.com/moltf/thc/internal/scanner"
)

const version = "1.0.0"

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func banner() {
	fmt.Print(`
████████╗██╗  ██╗ ██████╗
╚══██╔══╝██║  ██║██╔════╝
   ██║   ███████║██║
   ██║   ██╔══██║██║
   ██║   ██║  ██║╚██████╗
   ╚═╝   ╚═╝  ╚═╝ ╚═════╝

              Moltf
`)
}

func usage() {
	fmt.Printf(`THC %s

Usage:
  thc -ip 8.8.8.8
  thc -ip 8.8.8.0/24 -o domains.txt
  thc -i 8.8.8.8 -i 1.1.1.1 -c 20
  thc -l ips.txt -c 20 -o domains.txt
  cat ips.txt | thc -c 20

Options:
`, version)

	flag.PrintDefaults()
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return true
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func readTargets(listFile string, ips stringList, single string) ([]string, error) {
	var values []string

	values = append(values, ips...)

	if single != "" {
		values = append(values, single)
	}

	if listFile != "" {
		file, err := os.Open(listFile)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			values = append(values, scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	if len(values) == 0 && !isTerminal(os.Stdin) {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			values = append(values, scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	seen := make(map[string]struct{}, len(values))
	targets := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)

		if value == "" {
			continue
		}

		if strings.HasPrefix(value, "#") {
			continue
		}

		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		targets = append(targets, value)
	}

	return targets, nil
}

func main() {
	cfg := config.Default()

	var ips stringList

	flag.Usage = usage

	flag.Var(
		&ips,
		"i",
		"IP/CIDR (repeatable)",
	)

	ip := flag.String(
		"ip",
		"",
		"IP or CIDR",
	)

	list := flag.String(
		"l",
		"",
		"file containing IPs/CIDRs",
	)

	flag.StringVar(
		&cfg.Output,
		"o",
		"",
		"output file",
	)

	flag.StringVar(
		&cfg.Format,
		"format",
		"txt",
		"output format: txt, json, jsonl, csv",
	)

	flag.IntVar(
		&cfg.Workers,
		"c",
		10,
		"concurrent workers",
	)

	flag.IntVar(
		&cfg.Retries,
		"retries",
		3,
		"retries for transient failures",
	)

	flag.DurationVar(
		&cfg.Backoff,
		"backoff",
		time.Second,
		"initial retry backoff",
	)

	flag.DurationVar(
		&cfg.Timeout,
		"timeout",
		30*time.Second,
		"HTTP timeout",
	)

	flag.Float64Var(
		&cfg.Rate,
		"rate",
		0,
		"maximum requests per second, 0 disables limit",
	)

	flag.BoolVar(
		&cfg.Resume,
		"resume",
		false,
		"append to existing output",
	)

	flag.BoolVar(
		&cfg.Quiet,
		"q",
		false,
		"quiet mode",
	)

	flag.BoolVar(
		&cfg.NoProgress,
		"no-progress",
		false,
		"disable progress display",
	)

	flag.StringVar(
		&cfg.APIURL,
		"api",
		api.DefaultURL,
		"lookup API endpoint",
	)

	flag.StringVar(
		&cfg.UserAgent,
		"user-agent",
		"thc/"+version,
		"HTTP User-Agent",
	)

	versionFlag := flag.Bool(
		"version",
		false,
		"show version",
	)

	flag.Parse()

	if *versionFlag {
		fmt.Println("thc", version)
		return
	}

	if cfg.Workers < 1 {
		fmt.Fprintln(os.Stderr, "[-] workers must be >= 1")
		os.Exit(1)
	}

	if cfg.Retries < 0 {
		fmt.Fprintln(os.Stderr, "[-] retries must be >= 0")
		os.Exit(1)
	}

	if cfg.Rate < 0 {
		fmt.Fprintln(os.Stderr, "[-] rate must be >= 0")
		os.Exit(1)
	}

	targets, err := readTargets(
		*list,
		ips,
		*ip,
	)

	if err != nil {
		fmt.Fprintln(os.Stderr, "[-]", err)
		os.Exit(1)
	}

	if len(targets) == 0 {
		banner()
		usage()
		os.Exit(1)
	}

	if !cfg.Quiet {
		banner()
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	client := api.NewClient(
		api.ClientConfig{
			URL:       cfg.APIURL,
			Timeout:   cfg.Timeout,
			Retries:   cfg.Retries,
			Backoff:   cfg.Backoff,
			Rate:      cfg.Rate,
			UserAgent: cfg.UserAgent,
		},
	)

	writer, err := output.New(
		output.Config{
			Path:   cfg.Output,
			Format: cfg.Format,
			Resume: cfg.Resume,
		},
	)

	if err != nil {
		fmt.Fprintln(os.Stderr, "[-]", err)
		os.Exit(1)
	}

	defer writer.Close()

	s := scanner.New(
		client,
		writer,
		scanner.Config{
			Workers:    cfg.Workers,
			Quiet:      cfg.Quiet,
			NoProgress: cfg.NoProgress,
		},
	)

	stats := s.Run(
		ctx,
		targets,
	)

	if ctx.Err() != nil {
		fmt.Fprintln(
			os.Stderr,
			"[!] interrupted",
		)
	}

	if !cfg.Quiet {
		fmt.Printf("\n[+] Targets: %d\n", stats.Targets)
		fmt.Printf("[+] Processed: %d\n", stats.Processed)
		fmt.Printf("[+] Domains: %d\n", stats.Domains)
		fmt.Printf("[+] Duplicates: %d\n", stats.Duplicates)
		fmt.Printf("[+] Requests: %d\n", stats.Requests)
		fmt.Printf("[+] Errors: %d\n", stats.Errors)
		fmt.Printf("[+] Duration: %s\n",
			stats.Duration.Round(time.Millisecond),
		)

		if cfg.Output != "" {
			fmt.Printf("[+] Saved: %s\n", cfg.Output)
		}
	}
}
