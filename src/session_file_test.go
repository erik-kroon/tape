package tape_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
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
