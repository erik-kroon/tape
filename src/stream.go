package tape

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type Stream interface {
	Next() (Event, error)
	Close() error
}

type startAtSeeker interface {
	SeekStartAt(StartAt) (bool, error)
}

func OpenStream(path string) (Stream, error) {
	return OpenStreamWithCodecs(path)
}

func OpenStreams(paths []string) (Stream, error) {
	return OpenStreamsWithCodecs(paths)
}

func OpenStreamWithCodecs(path string, codecs ...EventCodec) (Stream, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".csv":
		return OpenCSVStream(path)
	case ".parquet":
		return OpenParquetStream(path)
	case ".tape", ".jsonl":
		return OpenJSONLStreamWithCodecs(path, codecs...)
	default:
		return nil, fmt.Errorf("unsupported input format %q", ext)
	}
}

func OpenStreamsWithCodecs(paths []string, codecs ...EventCodec) (Stream, error) {
	if len(paths) == 0 {
		return nil, errors.New("open stream error: at least one input path is required")
	}
	if len(paths) == 1 {
		return OpenStreamWithCodecs(paths[0], codecs...)
	}

	streams := make([]Stream, 0, len(paths))
	for _, path := range paths {
		stream, err := OpenStreamWithCodecs(path, codecs...)
		if err != nil {
			for _, opened := range streams {
				_ = opened.Close()
			}
			return nil, err
		}
		streams = append(streams, stream)
	}

	return newMergedStream(streams)
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
