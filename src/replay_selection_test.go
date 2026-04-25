package tape

import (
	"io"
	"testing"
	"time"
)

func TestReplaySelectionAppliesSeekStartAtFilterAndSelectedIndex(t *testing.T) {
	stream := &selectionSeekStream{
		events: []Event{
			Tick{Time: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC), Sym: "ERICB", Price: 92.5, Seq: 1},
			Tick{Time: time.Date(2026, 4, 24, 9, 30, 1, 0, time.UTC), Sym: "VOLV", Price: 301.0, Seq: 2},
			Tick{Time: time.Date(2026, 4, 24, 9, 30, 2, 0, time.UTC), Sym: "ERICB", Price: 92.7, Seq: 3},
			Tick{Time: time.Date(2026, 4, 24, 9, 30, 3, 0, time.UTC), Sym: "ERICB", Price: 92.8, Seq: 4},
		},
	}

	selection, err := NewReplaySelection(stream, Config{
		Mode: MaxSpeedMode,
		Filter: Filter{
			Symbols: []string{"ericb"},
		},
		StartAt: StartAt{Sequence: 3},
	})
	if err != nil {
		t.Fatalf("new replay selection: %v", err)
	}

	first, err := selection.Next()
	if err != nil {
		t.Fatalf("first next: %v", err)
	}
	if first.Index != 0 {
		t.Fatalf("first index = %d, want 0", first.Index)
	}
	if first.Event.Sequence() != 3 {
		t.Fatalf("first sequence = %d, want 3", first.Event.Sequence())
	}

	second, err := selection.Next()
	if err != nil {
		t.Fatalf("second next: %v", err)
	}
	if second.Index != 1 {
		t.Fatalf("second index = %d, want 1", second.Index)
	}
	if second.Event.Sequence() != 4 {
		t.Fatalf("second sequence = %d, want 4", second.Event.Sequence())
	}

	if _, err := selection.Next(); err != io.EOF {
		t.Fatalf("third next error = %v, want EOF", err)
	}
	if stream.seekCalls != 1 {
		t.Fatalf("seek calls = %d, want 1", stream.seekCalls)
	}
}

type selectionSeekStream struct {
	events    []Event
	index     int
	seekCalls int
	closed    bool
}

func (s *selectionSeekStream) Next() (Event, error) {
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *selectionSeekStream) Close() error {
	s.closed = true
	return nil
}

func (s *selectionSeekStream) SeekStartAt(startAt StartAt) (bool, error) {
	s.seekCalls++
	for index, event := range s.events {
		if startAt.Reached(event) {
			s.index = index
			return true, nil
		}
	}
	s.index = len(s.events)
	return true, nil
}
