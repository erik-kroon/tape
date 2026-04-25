package tape

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type csvStream struct {
	file    *os.File
	reader  *csv.Reader
	headers map[string]int
	kind    string
}

func OpenCSVStream(path string) (Stream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	headerRow, err := reader.Read()
	if err != nil {
		file.Close()
		return nil, err
	}

	headers := map[string]int{}
	for index, raw := range headerRow {
		headers[strings.ToLower(strings.TrimSpace(raw))] = index
	}

	stream := &csvStream{
		file:    file,
		reader:  reader,
		headers: headers,
		kind:    tapeEventSchema.InferKind(csvFieldLookup(headers)),
	}
	if stream.kind == "" {
		file.Close()
		return nil, fmt.Errorf("decode error: could not infer CSV event type from headers")
	}

	return stream, nil
}

func (s *csvStream) Next() (Event, error) {
	record, err := s.reader.Read()
	if err != nil {
		return nil, err
	}

	return tapeEventSchema.Decode(s.kind, csvRecordValues{
		headers: s.headers,
		record:  record,
	})
}

func (s *csvStream) Close() error {
	return s.file.Close()
}

type csvFieldLookup map[string]int

func (l csvFieldLookup) Has(alias string) bool {
	_, ok := l[alias]
	return ok
}

type csvRecordValues struct {
	headers map[string]int
	record  []string
}

func (v csvRecordValues) Has(alias string) bool {
	_, ok := v.headers[alias]
	return ok
}

func (v csvRecordValues) Time(required bool, aliases ...string) (time.Time, error) {
	value := v.String(aliases...)
	if value == "" {
		if required {
			return time.Time{}, fmt.Errorf("decode error: missing %s", aliases[0])
		}
		return time.Time{}, nil
	}

	for _, layout := range supportedTimeLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("decode error: invalid timestamp %q", value)
}

func (v csvRecordValues) String(aliases ...string) string {
	for _, alias := range aliases {
		index, ok := v.headers[alias]
		if !ok || index >= len(v.record) {
			continue
		}
		return strings.TrimSpace(v.record[index])
	}
	return ""
}

func (v csvRecordValues) Float(required bool, aliases ...string) (float64, error) {
	raw := v.String(aliases...)
	if raw == "" {
		if required {
			return 0, fmt.Errorf("decode error: missing %s", aliases[0])
		}
		return 0, nil
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("decode error: invalid %s %q", aliases[0], raw)
	}
	return value, nil
}

func (v csvRecordValues) Int(required bool, aliases ...string) (int64, error) {
	raw := v.String(aliases...)
	if raw == "" {
		if required {
			return 0, fmt.Errorf("decode error: missing %s", aliases[0])
		}
		return 0, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decode error: invalid %s %q", aliases[0], raw)
	}
	return value, nil
}
