package strategy

import (
	"fmt"
	"time"

	tape "github.com/erik-kroon/tape/src"
)

// ReplayRange defines an inclusive replay window.
type ReplayRange struct {
	Start time.Time
	End   time.Time
}

func (r ReplayRange) Active() bool {
	return !r.Start.IsZero() || !r.End.IsZero()
}

func (r ReplayRange) Contains(timestamp time.Time) bool {
	if !r.Start.IsZero() && timestamp.Before(r.Start) {
		return false
	}
	if !r.End.IsZero() && timestamp.After(r.End) {
		return false
	}
	return true
}

func (r ReplayRange) validate(name string) error {
	if !r.Start.IsZero() && !r.End.IsZero() && r.Start.After(r.End) {
		return fmt.Errorf(
			"strategy: %s start time %s is after end time %s",
			name,
			r.Start.Format(time.RFC3339Nano),
			r.End.Format(time.RFC3339Nano),
		)
	}
	return nil
}

// ReplayRanges separates indicator warmup from the measured assertion window.
type ReplayRanges struct {
	Warmup   ReplayRange
	Measured ReplayRange
}

func (r ReplayRanges) Active() bool {
	return r.Warmup.Active() || r.Measured.Active()
}

func (r ReplayRanges) validate() error {
	if err := r.Warmup.validate("warmup range"); err != nil {
		return err
	}
	if err := r.Measured.validate("measured range"); err != nil {
		return err
	}
	if r.Warmup.Active() && !r.Measured.Active() {
		return fmt.Errorf("strategy: measured range is required when warmup range is set")
	}
	if !r.Warmup.Start.IsZero() && !r.Measured.Start.IsZero() && r.Warmup.Start.After(r.Measured.Start) {
		return fmt.Errorf("strategy: warmup range start time must be before or equal to measured range start time")
	}
	return nil
}

func (r ReplayRanges) combinedFilter(filter tape.Filter) (tape.Filter, error) {
	if err := r.validate(); err != nil {
		return filter, err
	}
	if !r.Active() {
		return filter, nil
	}
	if !filter.StartTime.IsZero() || !filter.EndTime.IsZero() {
		return filter, fmt.Errorf("strategy: replay ranges cannot be combined with config filter start/end time")
	}

	start, ok := earliestTime(r.Warmup.Start, r.Measured.Start)
	if ok {
		filter.StartTime = start
	}
	end, ok := latestTime(r.Warmup.End, r.Measured.End)
	if ok {
		filter.EndTime = end
	}
	return filter, nil
}

func (r ReplayRanges) decorateContext(ctx tape.Context, event tape.Event, measuredIndex *int) tape.Context {
	if !r.Measured.Active() {
		ctx.Measured = true
		ctx.MeasuredIndex = ctx.Index
		return ctx
	}
	if r.Measured.Contains(event.Timestamp()) {
		ctx.Measured = true
		ctx.MeasuredIndex = *measuredIndex
		*measuredIndex = *measuredIndex + 1
		return ctx
	}

	ctx.Measured = false
	ctx.MeasuredIndex = -1
	return ctx
}

func earliestTime(left time.Time, right time.Time) (time.Time, bool) {
	switch {
	case left.IsZero():
		return right, !right.IsZero()
	case right.IsZero():
		return left, true
	case left.Before(right):
		return left, true
	default:
		return right, true
	}
}

func latestTime(left time.Time, right time.Time) (time.Time, bool) {
	switch {
	case left.IsZero():
		return right, !right.IsZero()
	case right.IsZero():
		return left, true
	case left.After(right):
		return left, true
	default:
		return right, true
	}
}
