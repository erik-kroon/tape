package tape

import (
	"fmt"
	"time"
)

var tapeEventSchema = eventSchema{
	timeAliases:     []string{"timestamp", "time", "ts"},
	symbolAliases:   []string{"symbol", "ticker", "instrument"},
	sequenceAliases: []string{"seq", "sequence"},
	families: []eventFamily{
		{
			kind:     "bar",
			matchAll: []string{"open", "high", "low", "close"},
			decode: func(core decodedEventCore, values eventValueSource) (Event, error) {
				open, err := values.Float(true, "open")
				if err != nil {
					return nil, err
				}
				high, err := values.Float(true, "high")
				if err != nil {
					return nil, err
				}
				low, err := values.Float(true, "low")
				if err != nil {
					return nil, err
				}
				closeValue, err := values.Float(true, "close")
				if err != nil {
					return nil, err
				}
				volume, err := values.Float(false, "volume")
				if err != nil {
					return nil, err
				}

				return Bar{
					Time:   core.timestamp,
					Sym:    core.symbol,
					Open:   open,
					High:   high,
					Low:    low,
					Close:  closeValue,
					Volume: volume,
					Seq:    core.sequence,
				}, nil
			},
		},
		{
			kind:     "tick",
			matchAny: []string{"price", "last"},
			decode: func(core decodedEventCore, values eventValueSource) (Event, error) {
				price, err := values.Float(true, "price", "last")
				if err != nil {
					return nil, err
				}
				size, err := values.Float(false, "size", "qty", "quantity", "volume")
				if err != nil {
					return nil, err
				}

				return Tick{
					Time:  core.timestamp,
					Sym:   core.symbol,
					Price: price,
					Size:  size,
					Seq:   core.sequence,
				}, nil
			},
		},
	},
}

var supportedTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

type eventSchema struct {
	timeAliases     []string
	symbolAliases   []string
	sequenceAliases []string
	families        []eventFamily
}

type eventFamily struct {
	kind     string
	matchAll []string
	matchAny []string
	decode   func(decodedEventCore, eventValueSource) (Event, error)
}

type decodedEventCore struct {
	timestamp time.Time
	symbol    string
	sequence  int64
}

type eventFieldLookup interface {
	Has(alias string) bool
}

type eventValueSource interface {
	eventFieldLookup
	Time(required bool, aliases ...string) (time.Time, error)
	String(aliases ...string) string
	Float(required bool, aliases ...string) (float64, error)
	Int(required bool, aliases ...string) (int64, error)
}

func (s eventSchema) InferKind(fields eventFieldLookup) string {
	for _, family := range s.families {
		if len(family.matchAll) > 0 && hasAllFields(fields, family.matchAll...) {
			return family.kind
		}
		if len(family.matchAny) > 0 && hasAnyField(fields, family.matchAny...) {
			return family.kind
		}
	}
	return ""
}

func (s eventSchema) Decode(kind string, values eventValueSource) (Event, error) {
	core, err := s.decodeCore(values)
	if err != nil {
		return nil, err
	}

	for _, family := range s.families {
		if family.kind == kind {
			return family.decode(core, values)
		}
	}

	return nil, fmt.Errorf("decode error: unsupported event type %q", kind)
}

func (s eventSchema) decodeCore(values eventValueSource) (decodedEventCore, error) {
	timestamp, err := values.Time(true, s.timeAliases...)
	if err != nil {
		return decodedEventCore{}, err
	}
	sequence, err := values.Int(false, s.sequenceAliases...)
	if err != nil {
		return decodedEventCore{}, err
	}

	return decodedEventCore{
		timestamp: timestamp,
		symbol:    values.String(s.symbolAliases...),
		sequence:  sequence,
	}, nil
}

func hasAllFields(fields eventFieldLookup, aliases ...string) bool {
	for _, alias := range aliases {
		if !fields.Has(alias) {
			return false
		}
	}
	return true
}

func hasAnyField(fields eventFieldLookup, aliases ...string) bool {
	for _, alias := range aliases {
		if fields.Has(alias) {
			return true
		}
	}
	return false
}
