# THC

Fast IP/CIDR to domain discovery CLI.

Built in Go for speed, concurrency and easy deployment.

## Features

- Single IP input
- CIDR input
- Multiple targets
- File input
- stdin support
- Target deduplication
- Domain deduplication
- Concurrent workers
- Rate limiting
- Automatic retries
- Exponential backoff
- 429 / 5xx handling
- Pagination
- Graceful Ctrl+C
- Incremental output
- TXT output
- JSON output
- JSONL output
- CSV output
- Progress statistics
- Custom API endpoint
- Custom User-Agent
- Cross-platform builds
- Unit tests
- GitHub Actions

## Installation

### From source

```bash
git clone https://github.com/moltf/thc.git
cd thc
go build -trimpath -ldflags="-s -w" -o thc .
