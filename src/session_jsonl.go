package tape

import (
	"bufio"
	"io"
	"os"
	"sync"
)

type Recorder struct {
	mu           sync.Mutex
	path         string
	file         *os.File
	writer       *bufio.Writer
	codecs       eventCodecRegistry
	bytesWritten int64
	indexEntries []sessionIndexEntry
}

func NewRecorder(path string) (*Recorder, error) {
	return NewRecorderWithCodecs(path)
}

func NewRecorderWithCodecs(path string, codecs ...EventCodec) (*Recorder, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	registry, err := newEventCodecRegistry(codecs)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &Recorder{
		path:   path,
		file:   file,
		writer: bufio.NewWriter(file),
		codecs: registry,
	}, nil
}

func (r *Recorder) Middleware() Middleware {
	return func(next EventHandler) EventHandler {
		return func(ctx Context, event Event) error {
			if err := r.Write(ctx, event); err != nil {
				return err
			}
			return next(ctx, event)
		}
	}
}

func (r *Recorder) Write(ctx Context, event Event) error {
	return r.Record(ctx.Index, event)
}

func (r *Recorder) Record(index int, event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.codecs.Marshal(index, event)
	if err != nil {
		return err
	}
	r.indexEntries = append(r.indexEntries, newSessionIndexEntry(r.bytesWritten, index, event))
	if _, err := r.writer.Write(record); err != nil {
		return err
	}
	if err := r.writer.WriteByte('\n'); err != nil {
		return err
	}
	r.bytesWritten += int64(len(record) + 1)
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
	if err := r.file.Close(); err != nil {
		return err
	}
	return writeSessionIndex(r.path, r.indexEntries)
}

type jsonlStream struct {
	file    *os.File
	scanner *bufio.Scanner
	codecs  eventCodecRegistry
	index   *sessionIndex
}

func OpenJSONLStream(path string) (Stream, error) {
	return OpenJSONLStreamWithCodecs(path)
}

func OpenJSONLStreamWithCodecs(path string, codecs ...EventCodec) (Stream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	registry, err := newEventCodecRegistry(codecs)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &jsonlStream{
		file:    file,
		scanner: bufio.NewScanner(file),
		codecs:  registry,
		index:   loadSessionIndex(path),
	}, nil
}

func (s *jsonlStream) Next() (Event, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	record, err := decodeSessionRecord(s.scanner.Bytes())
	if err != nil {
		return nil, err
	}
	return s.codecs.Decode(record)
}

func (s *jsonlStream) SeekStartAt(startAt StartAt) (bool, error) {
	if s.index == nil || !startAt.Active() {
		return false, nil
	}

	if _, err := s.file.Seek(s.index.offsetFor(startAt), io.SeekStart); err != nil {
		return false, err
	}
	s.scanner = bufio.NewScanner(s.file)
	return true, nil
}

func (s *jsonlStream) Close() error {
	return s.file.Close()
}
