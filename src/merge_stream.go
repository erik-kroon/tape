package tape

import (
	"container/heap"
	"errors"
	"io"
)

type mergedStream struct {
	sources     []mergedStreamSource
	queue       mergedStreamQueue
	initialized bool
}

type mergedStreamSource struct {
	stream Stream
}

type mergedStreamItem struct {
	source int
	event  Event
}

type mergedStreamQueue []mergedStreamItem

func newMergedStream(streams []Stream) (*mergedStream, error) {
	if len(streams) == 0 {
		return nil, errors.New("open stream error: at least one input path is required")
	}

	sources := make([]mergedStreamSource, len(streams))
	for index, stream := range streams {
		sources[index] = mergedStreamSource{stream: stream}
	}

	return &mergedStream{sources: sources}, nil
}

func (s *mergedStream) Next() (Event, error) {
	if err := s.initialize(); err != nil {
		return nil, err
	}
	if len(s.queue) == 0 {
		return nil, io.EOF
	}

	item := heap.Pop(&s.queue).(mergedStreamItem)
	if err := s.enqueueNext(item.source); err != nil {
		return nil, err
	}
	return item.event, nil
}

func (s *mergedStream) Close() error {
	var err error
	for _, source := range s.sources {
		err = errors.Join(err, source.stream.Close())
	}
	return err
}

func (s *mergedStream) SeekStartAt(startAt StartAt) (bool, error) {
	if s.initialized {
		return false, nil
	}

	seekedAll := true
	for _, source := range s.sources {
		seeker, ok := source.stream.(startAtSeeker)
		if !ok {
			seekedAll = false
			continue
		}

		seeked, err := seeker.SeekStartAt(startAt)
		if err != nil {
			return false, err
		}
		if !seeked {
			seekedAll = false
		}
	}

	return seekedAll, nil
}

func (s *mergedStream) initialize() error {
	if s.initialized {
		return nil
	}
	s.initialized = true

	for index := range s.sources {
		if err := s.enqueueNext(index); err != nil {
			return err
		}
	}
	return nil
}

func (s *mergedStream) enqueueNext(sourceIndex int) error {
	event, err := s.sources[sourceIndex].stream.Next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	heap.Push(&s.queue, mergedStreamItem{
		source: sourceIndex,
		event:  event,
	})
	return nil
}

func (q mergedStreamQueue) Len() int {
	return len(q)
}

func (q mergedStreamQueue) Less(i int, j int) bool {
	left := q[i]
	right := q[j]

	if left.event.Timestamp().Before(right.event.Timestamp()) {
		return true
	}
	if right.event.Timestamp().Before(left.event.Timestamp()) {
		return false
	}
	if left.event.Sequence() != right.event.Sequence() {
		return left.event.Sequence() < right.event.Sequence()
	}
	return left.source < right.source
}

func (q mergedStreamQueue) Swap(i int, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *mergedStreamQueue) Push(value any) {
	*q = append(*q, value.(mergedStreamItem))
}

func (q *mergedStreamQueue) Pop() any {
	old := *q
	last := len(old) - 1
	item := old[last]
	*q = old[:last]
	return item
}
