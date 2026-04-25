package tape

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEventSchemaInfersAndDecodesTick(t *testing.T) {
	source := decodeStub{
		times: map[string]time.Time{
			"ts": time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
		},
		strings: map[string]string{
			"ticker": " ERICB ",
		},
		floats: map[string]float64{
			"last": 92.5,
			"qty":  100,
		},
		ints: map[string]int64{
			"sequence": 7,
		},
	}

	kind := tapeEventSchema.InferKind(source)
	if kind != "tick" {
		t.Fatalf("kind = %q, want tick", kind)
	}

	event, err := tapeEventSchema.Decode(kind, source)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	tick, ok := event.(Tick)
	if !ok {
		t.Fatalf("event type = %T, want Tick", event)
	}
	if tick.Sym != "ERICB" {
		t.Fatalf("symbol = %q, want ERICB", tick.Sym)
	}
	if tick.Price != 92.5 || tick.Size != 100 || tick.Seq != 7 {
		t.Fatalf("tick = %#v, want decoded aliases", tick)
	}
}

func TestEventSchemaInfersAndDecodesBar(t *testing.T) {
	source := decodeStub{
		times: map[string]time.Time{
			"timestamp": time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC),
		},
		strings: map[string]string{
			"instrument": "ERICB",
		},
		floats: map[string]float64{
			"open":   92.4,
			"high":   92.8,
			"low":    92.2,
			"close":  92.7,
			"volume": 250,
		},
		ints: map[string]int64{
			"seq": 8,
		},
	}

	kind := tapeEventSchema.InferKind(source)
	if kind != "bar" {
		t.Fatalf("kind = %q, want bar", kind)
	}

	event, err := tapeEventSchema.Decode(kind, source)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	bar, ok := event.(Bar)
	if !ok {
		t.Fatalf("event type = %T, want Bar", event)
	}
	if bar.Sym != "ERICB" || bar.Open != 92.4 || bar.Close != 92.7 || bar.Volume != 250 || bar.Seq != 8 {
		t.Fatalf("bar = %#v, want decoded aliases", bar)
	}
}

type decodeStub struct {
	times   map[string]time.Time
	strings map[string]string
	floats  map[string]float64
	ints    map[string]int64
}

func (s decodeStub) Has(alias string) bool {
	if _, ok := s.times[alias]; ok {
		return true
	}
	if _, ok := s.strings[alias]; ok {
		return true
	}
	if _, ok := s.floats[alias]; ok {
		return true
	}
	_, ok := s.ints[alias]
	return ok
}

func (s decodeStub) Time(required bool, aliases ...string) (time.Time, error) {
	for _, alias := range aliases {
		if value, ok := s.times[alias]; ok {
			return value, nil
		}
	}
	if required {
		return time.Time{}, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return time.Time{}, nil
}

func (s decodeStub) String(aliases ...string) string {
	for _, alias := range aliases {
		if value, ok := s.strings[alias]; ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s decodeStub) Float(required bool, aliases ...string) (float64, error) {
	for _, alias := range aliases {
		if value, ok := s.floats[alias]; ok {
			return value, nil
		}
	}
	if required {
		return 0, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return 0, nil
}

func (s decodeStub) Int(required bool, aliases ...string) (int64, error) {
	for _, alias := range aliases {
		if value, ok := s.ints[alias]; ok {
			return value, nil
		}
	}
	if required {
		return 0, fmt.Errorf("decode error: missing %s", aliases[0])
	}
	return 0, nil
}
