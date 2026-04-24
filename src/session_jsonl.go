package tape

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type Recorder struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
}

func NewRecorder(path string) (*Recorder, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	return &Recorder{
		file:   file,
		writer: bufio.NewWriter(file),
	}, nil
}

func (r *Recorder) Middleware() Middleware {
	return func(next EventHandler) EventHandler {
		return func(ctx Context, event Event) error {
			if err := r.Record(ctx.Index, event); err != nil {
				return err
			}
			return next(ctx, event)
		}
	}
}

func (r *Recorder) Record(index int, event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := marshalSessionRecord(index, event)
	if err != nil {
		return err
	}
	if _, err := r.writer.Write(record); err != nil {
		return err
	}
	if err := r.writer.WriteByte('\n'); err != nil {
		return err
	}
	return r.writer.Flush()
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writer != nil {
		if err := r.writer.Flush(); err != nil {
			r.file.Close()
			return err
		}
	}
	return r.file.Close()
}

func marshalSessionRecord(index int, event Event) ([]byte, error) {
	switch typed := event.(type) {
	case Tick:
		return json.Marshal(sessionRecord{Type: typed.Type(), Tick: &typed, Index: index})
	case Bar:
		return json.Marshal(sessionRecord{Type: typed.Type(), Bar: &typed, Index: index})
	default:
		return nil, fmt.Errorf("sink error: unsupported event type %T", event)
	}
}

type jsonlStream struct {
	file    *os.File
	scanner *bufio.Scanner
}

func OpenJSONLStream(path string) (Stream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return &jsonlStream{
		file:    file,
		scanner: bufio.NewScanner(file),
	}, nil
}

func (s *jsonlStream) Next() (Event, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	var record sessionRecord
	if err := json.Unmarshal(s.scanner.Bytes(), &record); err != nil {
		return nil, fmt.Errorf("decode error: invalid session record: %w", err)
	}

	switch record.Type {
	case "tick":
		if record.Tick == nil {
			return nil, fmt.Errorf("decode error: tick record missing payload")
		}
		return *record.Tick, nil
	case "bar":
		if record.Bar == nil {
			return nil, fmt.Errorf("decode error: bar record missing payload")
		}
		return *record.Bar, nil
	default:
		return nil, fmt.Errorf("decode error: unknown event type %q", record.Type)
	}
}

func (s *jsonlStream) Close() error {
	return s.file.Close()
}
