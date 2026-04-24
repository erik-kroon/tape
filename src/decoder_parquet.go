package tape

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
)

type parquetStream struct {
	file   *os.File
	reader *parquet.GenericReader[parquetEventRow]
	kind   string
}

type parquetEventRow struct {
	Timestamp  *time.Time `parquet:"timestamp,optional,timestamp(nanosecond)"`
	Time       *time.Time `parquet:"time,optional,timestamp(nanosecond)"`
	TS         *time.Time `parquet:"ts,optional,timestamp(nanosecond)"`
	Symbol     *string    `parquet:"symbol,optional"`
	Ticker     *string    `parquet:"ticker,optional"`
	Instrument *string    `parquet:"instrument,optional"`
	Price      *float64   `parquet:"price,optional"`
	Last       *float64   `parquet:"last,optional"`
	Size       *float64   `parquet:"size,optional"`
	Qty        *float64   `parquet:"qty,optional"`
	Quantity   *float64   `parquet:"quantity,optional"`
	Volume     *float64   `parquet:"volume,optional"`
	Seq        *int64     `parquet:"seq,optional"`
	Sequence   *int64     `parquet:"sequence,optional"`
	Open       *float64   `parquet:"open,optional"`
	High       *float64   `parquet:"high,optional"`
	Low        *float64   `parquet:"low,optional"`
	Close      *float64   `parquet:"close,optional"`
}

func OpenParquetStream(path string) (Stream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	parquetFile, err := parquet.OpenFile(file, info.Size())
	if err != nil {
		file.Close()
		return nil, err
	}

	headers := map[string]int{}
	for index, column := range parquetFile.Schema().Columns() {
		if len(column) == 0 {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(column[len(column)-1]))] = index
	}

	stream := &parquetStream{
		file:   file,
		reader: parquet.NewGenericReader[parquetEventRow](file),
		kind:   inferCSVKind(headers),
	}
	if stream.kind == "" {
		file.Close()
		return nil, fmt.Errorf("decode error: could not infer Parquet event type from columns")
	}

	return stream, nil
}

func (s *parquetStream) Next() (Event, error) {
	rows := make([]parquetEventRow, 1)
	count, err := s.reader.Read(rows)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if count == 0 {
		return nil, io.EOF
	}

	row := rows[0]
	timestamp, err := row.timeValue(true, "timestamp", "time", "ts")
	if err != nil {
		return nil, err
	}
	symbol := row.stringValue("symbol", "ticker", "instrument")
	sequence, err := row.intValue(false, "seq", "sequence")
	if err != nil {
		return nil, err
	}

	switch s.kind {
	case "tick":
		price, err := row.floatValue(true, "price", "last")
		if err != nil {
			return nil, err
		}
		size, err := row.floatValue(false, "size", "qty", "quantity", "volume")
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
		open, err := row.floatValue(true, "open")
		if err != nil {
			return nil, err
		}
		high, err := row.floatValue(true, "high")
		if err != nil {
			return nil, err
		}
		low, err := row.floatValue(true, "low")
		if err != nil {
			return nil, err
		}
		closeValue, err := row.floatValue(true, "close")
		if err != nil {
			return nil, err
		}
		volume, err := row.floatValue(false, "volume")
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
		return nil, fmt.Errorf("decode error: unsupported Parquet type %q", s.kind)
	}
}

func (s *parquetStream) Close() error {
	if err := s.reader.Close(); err != nil {
		s.file.Close()
		return err
	}
	return s.file.Close()
}

func (r parquetEventRow) timeValue(required bool, aliases ...string) (time.Time, error) {
	for _, alias := range aliases {
		switch alias {
		case "timestamp":
			if r.Timestamp != nil {
				return *r.Timestamp, nil
			}
		case "time":
			if r.Time != nil {
				return *r.Time, nil
			}
		case "ts":
			if r.TS != nil {
				return *r.TS, nil
			}
		}
	}

	if required {
		return time.Time{}, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return time.Time{}, nil
}

func (r parquetEventRow) stringValue(aliases ...string) string {
	for _, alias := range aliases {
		switch alias {
		case "symbol":
			if r.Symbol != nil {
				return strings.TrimSpace(*r.Symbol)
			}
		case "ticker":
			if r.Ticker != nil {
				return strings.TrimSpace(*r.Ticker)
			}
		case "instrument":
			if r.Instrument != nil {
				return strings.TrimSpace(*r.Instrument)
			}
		}
	}
	return ""
}

func (r parquetEventRow) floatValue(required bool, aliases ...string) (float64, error) {
	for _, alias := range aliases {
		switch alias {
		case "price":
			if r.Price != nil {
				return *r.Price, nil
			}
		case "last":
			if r.Last != nil {
				return *r.Last, nil
			}
		case "size":
			if r.Size != nil {
				return *r.Size, nil
			}
		case "qty":
			if r.Qty != nil {
				return *r.Qty, nil
			}
		case "quantity":
			if r.Quantity != nil {
				return *r.Quantity, nil
			}
		case "volume":
			if r.Volume != nil {
				return *r.Volume, nil
			}
		case "open":
			if r.Open != nil {
				return *r.Open, nil
			}
		case "high":
			if r.High != nil {
				return *r.High, nil
			}
		case "low":
			if r.Low != nil {
				return *r.Low, nil
			}
		case "close":
			if r.Close != nil {
				return *r.Close, nil
			}
		}
	}

	if required {
		return 0, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return 0, nil
}

func (r parquetEventRow) intValue(required bool, aliases ...string) (int64, error) {
	for _, alias := range aliases {
		switch alias {
		case "seq":
			if r.Seq != nil {
				return *r.Seq, nil
			}
		case "sequence":
			if r.Sequence != nil {
				return *r.Sequence, nil
			}
		}
	}

	if required {
		return 0, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return 0, nil
}
