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
		kind:    inferCSVKind(headers),
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

	timestamp, err := s.parseTime(record)
	if err != nil {
		return nil, err
	}
	symbol := s.stringValue(record, "symbol", "ticker", "instrument")
	sequence, err := s.intValue(record, false, "seq", "sequence")
	if err != nil {
		return nil, err
	}

	switch s.kind {
	case "tick":
		price, err := s.floatValue(record, true, "price", "last")
		if err != nil {
			return nil, err
		}
		size, err := s.floatValue(record, false, "size", "qty", "quantity", "volume")
		if err != nil {
			return nil, err
		}
		return Tick{
			Time:  timestamp,
			Sym:   symbol,
			Price: price,
			Size:  size,
			Seq:   sequence,
		}, nil
	case "bar":
		open, err := s.floatValue(record, true, "open")
		if err != nil {
			return nil, err
		}
		high, err := s.floatValue(record, true, "high")
		if err != nil {
			return nil, err
		}
		low, err := s.floatValue(record, true, "low")
		if err != nil {
			return nil, err
		}
		closeValue, err := s.floatValue(record, true, "close")
		if err != nil {
			return nil, err
		}
		volume, err := s.floatValue(record, false, "volume")
		if err != nil {
			return nil, err
		}
		return Bar{
			Time:   timestamp,
			Sym:    symbol,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeValue,
			Volume: volume,
			Seq:    sequence,
		}, nil
	default:
		return nil, fmt.Errorf("decode error: unsupported CSV type %q", s.kind)
	}
}

func (s *csvStream) Close() error {
	return s.file.Close()
}

func inferCSVKind(headers map[string]int) string {
	if hasAll(headers, "open", "high", "low", "close") {
		return "bar"
	}
	if hasAny(headers, "price", "last") {
		return "tick"
	}
	return ""
}

func hasAll(headers map[string]int, fields ...string) bool {
	for _, field := range fields {
		if _, ok := headers[field]; !ok {
			return false
		}
	}
	return true
}

func hasAny(headers map[string]int, fields ...string) bool {
	for _, field := range fields {
		if _, ok := headers[field]; ok {
			return true
		}
	}
	return false
}

func (s *csvStream) parseTime(record []string) (time.Time, error) {
	value := s.stringValue(record, "timestamp", "time", "ts")
	if value == "" {
		return time.Time{}, fmt.Errorf("decode error: missing timestamp column")
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("decode error: invalid timestamp %q", value)
}

func (s *csvStream) stringValue(record []string, aliases ...string) string {
	for _, alias := range aliases {
		index, ok := s.headers[alias]
		if !ok || index >= len(record) {
			continue
		}
		return strings.TrimSpace(record[index])
	}
	return ""
}

func (s *csvStream) floatValue(record []string, required bool, aliases ...string) (float64, error) {
	raw := s.stringValue(record, aliases...)
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

func (s *csvStream) intValue(record []string, required bool, aliases ...string) (int64, error) {
	raw := s.stringValue(record, aliases...)
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
