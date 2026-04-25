package tape

type SelectedEvent struct {
	Index int
	Event Event
}

type ReplaySelection struct {
	stream   Stream
	config   Config
	started  bool
	prev     Event
	selected int
}

func OpenReplaySelection(path string, config Config) (*ReplaySelection, error) {
	normalized := config.normalized()
	if err := normalized.validate(); err != nil {
		return nil, err
	}

	stream, err := OpenStreamWithCodecs(path, normalized.EventCodecs...)
	if err != nil {
		return nil, err
	}

	selection, err := NewReplaySelection(stream, normalized)
	if err != nil {
		stream.Close()
		return nil, err
	}
	return selection, nil
}

func NewReplaySelection(stream Stream, config Config) (*ReplaySelection, error) {
	normalized := config.normalized()
	if err := normalized.validate(); err != nil {
		return nil, err
	}

	selection := &ReplaySelection{
		stream:  stream,
		config:  normalized,
		started: !normalized.StartAt.Active(),
	}

	if seeker, ok := stream.(startAtSeeker); ok && normalized.StartAt.Active() {
		seeked, err := seeker.SeekStartAt(normalized.StartAt)
		if err != nil {
			return nil, err
		}
		if seeked {
			selection.started = true
		}
	}

	return selection, nil
}

func (s *ReplaySelection) Next() (SelectedEvent, error) {
	for {
		event, err := s.stream.Next()
		if err != nil {
			return SelectedEvent{}, err
		}

		if err := validateOrdering(s.prev, event, s.config.Permissive); err != nil {
			return SelectedEvent{}, err
		}
		s.prev = event

		if !s.started {
			if !s.config.StartAt.Reached(event) {
				continue
			}
			s.started = true
		}

		if !s.config.Filter.Matches(event) {
			continue
		}

		selected := SelectedEvent{
			Index: s.selected,
			Event: event,
		}
		s.selected++
		return selected, nil
	}
}

func (s *ReplaySelection) Close() error {
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}
