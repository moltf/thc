package output

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Config struct {
	Path   string
	Format string
	Resume bool
}

type Record struct {
	Target string `json:"target"`
	Domain string `json:"domain"`
}

type Writer struct {
	mu          sync.Mutex
	file        *os.File
	buffer      *bufio.Writer
	format      string
	jsonRecords []Record
	csvWriter   *csv.Writer
}

func New(cfg Config) (*Writer, error) {
	format := strings.ToLower(cfg.Format)

	if format == "" {
		format = "txt"
	}

	switch format {
	case "txt", "json", "jsonl", "csv":
	default:
		return nil, fmt.Errorf(
			"unsupported format: %s",
			cfg.Format,
		)
	}

	writer := &Writer{
		format: format,
	}

	if cfg.Path == "" {
		return writer, nil
	}

	if dir := filepath.Dir(cfg.Path); dir != "." {
		if err := os.MkdirAll(
			dir,
			0755,
		); err != nil {
			return nil, err
		}
	}

	flags := os.O_CREATE | os.O_WRONLY

	if cfg.Resume {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(
		cfg.Path,
		flags,
		0644,
	)

	if err != nil {
		return nil, err
	}

	writer.file = file

	writer.buffer = bufio.NewWriterSize(
		file,
		64*1024,
	)

	if format == "csv" {
		writer.csvWriter = csv.NewWriter(
			writer.buffer,
		)

		if !cfg.Resume {
			if err := writer.csvWriter.Write(
				[]string{
					"target",
					"domain",
				},
			); err != nil {
				file.Close()
				return nil, err
			}

			writer.csvWriter.Flush()
		}
	}

	return writer, nil
}

func (w *Writer) Write(
	record Record,
) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		fmt.Println(record.Domain)
		return nil
	}

	switch w.format {
	case "txt":
		if _, err := fmt.Fprintln(
			w.buffer,
			record.Domain,
		); err != nil {
			return err
		}

	case "jsonl":
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}

		if _, err := w.buffer.Write(
			append(data, '\n'),
		); err != nil {
			return err
		}

	case "json":
		w.jsonRecords = append(
			w.jsonRecords,
			record,
		)

	case "csv":
		if err := w.csvWriter.Write(
			[]string{
				record.Target,
				record.Domain,
			},
		); err != nil {
			return err
		}

		w.csvWriter.Flush()

		if err := w.csvWriter.Error(); err != nil {
			return err
		}
	}

	return w.buffer.Flush()
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	if w.format == "json" {
		data, err := json.MarshalIndent(
			w.jsonRecords,
			"",
			"  ",
		)

		if err != nil {
			w.file.Close()
			return err
		}

		if _, err := w.buffer.Write(data); err != nil {
			w.file.Close()
			return err
		}

		if _, err := w.buffer.WriteString("\n"); err != nil {
			w.file.Close()
			return err
		}
	}

	if err := w.buffer.Flush(); err != nil {
		w.file.Close()
		return err
	}

	return w.file.Close()
}
