package config

import "time"

type Config struct {
	Output     string
	Format     string
	Workers    int
	Retries    int
	Backoff    time.Duration
	Timeout    time.Duration
	Rate       float64
	Resume     bool
	Quiet      bool
	NoProgress bool
	APIURL     string
	UserAgent  string
}

func Default() Config {
	return Config{
		Workers: 10,
		Retries: 3,
		Backoff: time.Second,
		Timeout: 30 * time.Second,
		Format:  "txt",
	}
}
