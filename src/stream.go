package tape

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type Stream interface {
	Next() (Event, error)
	Close() error
}

func OpenStream(path string) (Stream, error) {
	return OpenStreamWithCodecs(path)
}

func OpenStreamWithCodecs(path string, codecs ...EventCodec) (Stream, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".csv":
		return OpenCSVStream(path)
	case ".tape", ".jsonl":
		return OpenJSONLStreamWithCodecs(path, codecs...)
	default:
		return nil, fmt.Errorf("unsupported input format %q", ext)
	}
}

type sliceStream struct {
	events []Event
	index  int
}

func newSliceStream(events []Event) Stream {
	return &sliceStream{events: events}
}

func (s *sliceStream) Next() (Event, error) {
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *sliceStream) Close() error {
	return nil
}
