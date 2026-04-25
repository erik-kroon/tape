package tape_test

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tape "github.com/erik-kroon/tape/src"
)

func TestSessionFileRoundTripAndSeek(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(path)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}

	events := []tape.Tick{
		{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 93.12, Seq: 1},
		{Time: time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC), Sym: "ERICB", Price: 93.20, Seq: 2},
		{Time: time.Date(2026, 4, 24, 9, 30, 2, 0, time.UTC), Sym: "ERICB", Price: 93.25, Seq: 3},
	}
	for index, event := range events {
		if err := recorder.Record(index, event); err != nil {
			t.Fatalf("record %d: %v", index, err)
		}
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	stream, err := tape.OpenJSONLStream(path)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	seeker, ok := stream.(interface {
		SeekStartAt(tape.StartAt) (bool, error)
	})
	if !ok {
		t.Fatal("stream does not support start-at seeking")
	}
	seeked, err := seeker.SeekStartAt(tape.StartAt{Sequence: 2})
	if err != nil {
		t.Fatalf("seek start-at: %v", err)
	}
	if !seeked {
		t.Fatal("seeked = false, want true")
	}

	event, err := stream.Next()
	if err != nil {
		t.Fatalf("next after seek: %v", err)
	}
	tick, ok := event.(tape.Tick)
	if !ok {
		t.Fatalf("event type = %T, want tape.Tick", event)
	}
	if tick.Seq != 2 {
		t.Fatalf("seq = %d, want 2", tick.Seq)
	}
}

func TestSessionFileWritesMetadataHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(path)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	for index, event := range []tape.Event{
		tape.Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "VOLV", Price: 301.00, Seq: 1},
		tape.Bar{Time: time.Date(2026, 4, 24, 9, 31, 0, 0, time.UTC), Sym: "ERICB", Open: 93.00, High: 93.40, Low: 92.90, Close: 93.25, Seq: 2},
	} {
		if err := recorder.Record(index, event); err != nil {
			t.Fatalf("record %d: %v", index, err)
		}
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) != 3 {
		t.Fatalf("record lines = %d, want 3", len(lines))
	}

	var header struct {
		Meta struct {
			SchemaVersion       int      `json:"schema_version"`
			SymbolUniverse      []string `json:"symbol_universe"`
			EventFamilies       []string `json:"event_families"`
			TimezoneAssumptions []string `json:"timezone_assumptions"`
			SourceInfo          struct {
				Sources []struct {
					Path   string `json:"path"`
					Format string `json:"format"`
				} `json:"sources"`
			} `json:"source_info"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("unmarshal metadata header: %v", err)
	}

	if header.Meta.SchemaVersion != 1 {
		t.Fatalf("schema version = %d, want 1", header.Meta.SchemaVersion)
	}
	if got := strings.Join(header.Meta.SymbolUniverse, ","); got != "ERICB,VOLV" {
		t.Fatalf("symbol universe = %q, want ERICB,VOLV", got)
	}
	if got := strings.Join(header.Meta.EventFamilies, ","); got != "bar,tick" {
		t.Fatalf("event families = %q, want bar,tick", got)
	}
	if len(header.Meta.SourceInfo.Sources) != 0 {
		t.Fatalf("source count = %d, want 0", len(header.Meta.SourceInfo.Sources))
	}
	if got := strings.Join(header.Meta.TimezoneAssumptions, ","); got != "recorded timestamps preserve the offset carried by each event" {
		t.Fatalf("timezone assumptions = %q, want recorded event offset note", got)
	}
}

func TestSessionFileFallsBackWhenIndexIsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.tape")

	recorder, err := tape.NewRecorder(path)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	if err := recorder.Record(0, tape.Tick{
		Time:  time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
		Sym:   "ERICB",
		Price: 93.12,
		Seq:   1,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("rewrite recording: %v", err)
	}

	stream, err := tape.OpenJSONLStream(path)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	seeker, ok := stream.(interface {
		SeekStartAt(tape.StartAt) (bool, error)
	})
	if !ok {
		t.Fatal("stream does not support start-at seeking")
	}
	seeked, err := seeker.SeekStartAt(tape.StartAt{Sequence: 1})
	if err != nil {
		t.Fatalf("seek start-at: %v", err)
	}
	if seeked {
		t.Fatal("seeked = true, want stale index fallback")
	}

	_, err = stream.Next()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("next error = %v, want EOF", err)
	}
}
