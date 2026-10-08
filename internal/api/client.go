package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const DefaultURL = "https://ip.thc.org/api/v1/lookup"

const limit = 100

type ClientConfig struct {
	URL       string
	Timeout   time.Duration
	Retries   int
	Backoff   time.Duration
	Rate      float64
	UserAgent string
}

type Client struct {
	cfg    ClientConfig
	http   *http.Client
	rateMu sync.Mutex
	last   time.Time
}

type requestBody struct {
	IPAddress string `json:"ip_address"`
	PageState string `json:"page_state"`
	Limit     int    `json:"limit"`
}

type DomainRecord struct {
	Domain string `json:"domain"`
}

type Response struct {
	Domains       []DomainRecord `json:"domains"`
	NextPageState string         `json:"next_page_state"`
}

type Result struct {
	Domains  []string
	Requests int
}

func NewClient(cfg ClientConfig) *Client {
	if cfg.URL == "" {
		cfg.URL = DefaultURL
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}

	if cfg.Backoff <= 0 {
		cfg.Backoff = time.Second
	}

	if cfg.UserAgent == "" {
		cfg.UserAgent = "thc"
	}

	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (c *Client) waitRate(ctx context.Context) error {
	if c.cfg.Rate <= 0 {
		return nil
	}

	interval := time.Duration(
		float64(time.Second) / c.cfg.Rate,
	)

	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	wait := time.Until(
		c.last.Add(interval),
	)

	if wait > 0 {
		timer := time.NewTimer(wait)

		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()

		case <-timer.C:
		}
	}

	c.last = time.Now()

	return nil
}

func (c *Client) Lookup(
	ctx context.Context,
	target string,
) (Result, error) {
	var result Result

	pageState := ""

	for {
		body := requestBody{
			IPAddress: target,
			PageState: pageState,
			Limit:     limit,
		}

		raw, err := json.Marshal(body)
		if err != nil {
			return result, err
		}

		var response Response

		if err := c.do(
			ctx,
			raw,
			&response,
		); err != nil {
			return result, err
		}

		result.Requests++

		for _, item := range response.Domains {
			if item.Domain == "" {
				continue
			}

			result.Domains = append(
				result.Domains,
				item.Domain,
			)
		}

		if response.NextPageState == "" {
			break
		}

		pageState = response.NextPageState
	}

	return result, nil
}

func (c *Client) do(
	ctx context.Context,
	body []byte,
	dst *Response,
) error {
	var lastErr error

	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if err := c.waitRate(ctx); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			c.cfg.URL,
			bytes.NewReader(body),
		)

		if err != nil {
			return err
		}

		req.Header.Set(
			"Content-Type",
			"application/x-www-form-urlencoded",
		)

		req.Header.Set(
			"Accept",
			"application/json, */*",
		)

		req.Header.Set(
			"Origin",
			"https://ip.thc.org",
		)

		req.Header.Set(
			"Referer",
			"https://ip.thc.org/",
		)

		req.Header.Set(
			"User-Agent",
			c.cfg.UserAgent,
		)

		resp, err := c.http.Do(req)

		if err != nil {
			lastErr = err
		} else {
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()

			if readErr != nil {
				lastErr = readErr
			} else if resp.StatusCode == http.StatusOK {
				if err := json.Unmarshal(
					data,
					dst,
				); err != nil {
					return err
				}

				return nil
			} else {
				lastErr = fmt.Errorf(
					"HTTP %d",
					resp.StatusCode,
				)

				if !retryable(resp.StatusCode) {
					return lastErr
				}

				if value := resp.Header.Get(
					"Retry-After",
				); value != "" {
					if seconds, err := strconv.Atoi(value); err == nil &&
						seconds > 0 {

						delay := time.Duration(seconds) * time.Second

						if !sleep(ctx, delay) {
							return ctx.Err()
						}

						continue
					}
				}
			}
		}

		if attempt < c.cfg.Retries {
			delay := c.cfg.Backoff *
				time.Duration(1<<attempt)

			if delay > 30*time.Second {
				delay = 30 * time.Second
			}

			if !sleep(ctx, delay) {
				return ctx.Err()
			}
		}
	}

	return lastErr
}

func retryable(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func sleep(
	ctx context.Context,
	delay time.Duration,
) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false

	case <-timer.C:
		return true
	}
}
