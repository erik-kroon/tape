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
		kind:   tapeEventSchema.InferKind(parquetFieldLookup(headers)),
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

	return tapeEventSchema.Decode(s.kind, parquetRowValues{row: rows[0]})
}

func (s *parquetStream) Close() error {
	if err := s.reader.Close(); err != nil {
		s.file.Close()
		return err
	}
	return s.file.Close()
}

type parquetFieldLookup map[string]int

func (l parquetFieldLookup) Has(alias string) bool {
	_, ok := l[alias]
	return ok
}

type parquetRowValues struct {
	row parquetEventRow
}

func (v parquetRowValues) Has(alias string) bool {
	switch alias {
	case "timestamp":
		return v.row.Timestamp != nil
	case "time":
		return v.row.Time != nil
	case "ts":
		return v.row.TS != nil
	case "symbol":
		return v.row.Symbol != nil
	case "ticker":
		return v.row.Ticker != nil
	case "instrument":
		return v.row.Instrument != nil
	case "price":
		return v.row.Price != nil
	case "last":
		return v.row.Last != nil
	case "size":
		return v.row.Size != nil
	case "qty":
		return v.row.Qty != nil
	case "quantity":
		return v.row.Quantity != nil
	case "volume":
		return v.row.Volume != nil
	case "seq":
		return v.row.Seq != nil
	case "sequence":
		return v.row.Sequence != nil
	case "open":
		return v.row.Open != nil
	case "high":
		return v.row.High != nil
	case "low":
		return v.row.Low != nil
	case "close":
		return v.row.Close != nil
	default:
		return false
	}
}

func (v parquetRowValues) Time(required bool, aliases ...string) (time.Time, error) {
	for _, alias := range aliases {
		switch alias {
		case "timestamp":
			if v.row.Timestamp != nil {
				return *v.row.Timestamp, nil
			}
		case "time":
			if v.row.Time != nil {
				return *v.row.Time, nil
			}
		case "ts":
			if v.row.TS != nil {
				return *v.row.TS, nil
			}
		}
	}

	if required {
		return time.Time{}, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return time.Time{}, nil
}

func (v parquetRowValues) String(aliases ...string) string {
	for _, alias := range aliases {
		switch alias {
		case "symbol":
			if v.row.Symbol != nil {
				return strings.TrimSpace(*v.row.Symbol)
			}
		case "ticker":
			if v.row.Ticker != nil {
				return strings.TrimSpace(*v.row.Ticker)
			}
		case "instrument":
			if v.row.Instrument != nil {
				return strings.TrimSpace(*v.row.Instrument)
			}
		}
	}
	return ""
}

func (v parquetRowValues) Float(required bool, aliases ...string) (float64, error) {
	for _, alias := range aliases {
		switch alias {
		case "price":
			if v.row.Price != nil {
				return *v.row.Price, nil
			}
		case "last":
			if v.row.Last != nil {
				return *v.row.Last, nil
			}
		case "size":
			if v.row.Size != nil {
				return *v.row.Size, nil
			}
		case "qty":
			if v.row.Qty != nil {
				return *v.row.Qty, nil
			}
		case "quantity":
			if v.row.Quantity != nil {
				return *v.row.Quantity, nil
			}
		case "volume":
			if v.row.Volume != nil {
				return *v.row.Volume, nil
			}
		case "open":
			if v.row.Open != nil {
				return *v.row.Open, nil
			}
		case "high":
			if v.row.High != nil {
				return *v.row.High, nil
			}
		case "low":
			if v.row.Low != nil {
				return *v.row.Low, nil
			}
		case "close":
			if v.row.Close != nil {
				return *v.row.Close, nil
			}
		}
	}

	if required {
		return 0, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return 0, nil
}

func (v parquetRowValues) Int(required bool, aliases ...string) (int64, error) {
	for _, alias := range aliases {
		switch alias {
		case "seq":
			if v.row.Seq != nil {
				return *v.row.Seq, nil
			}
		case "sequence":
			if v.row.Sequence != nil {
				return *v.row.Sequence, nil
			}
		}
	}

	if required {
		return 0, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return 0, nil
}
