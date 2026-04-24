package tape

import (
	"fmt"
	"io"
	"os"
	"time"
)

type Mode string

const (
	RealTimeMode    Mode = "realtime"
	AcceleratedMode Mode = "accelerated"
	MaxSpeedMode    Mode = "max"
	StepMode        Mode = "step"
)

type Config struct {
	Mode        Mode
	Speed       float64
	Permissive  bool
	Filter      Filter
	StartAt     StartAt
	StepReader  io.Reader
	StepWriter  io.Writer
	EventCodecs []EventCodec
}

func (c Config) normalized() Config {
	if c.Mode == "" {
		c.Mode = MaxSpeedMode
	}
	if c.Speed <= 0 {
		c.Speed = 1
	}
	if c.StepReader == nil {
		c.StepReader = os.Stdin
	}
	if c.StepWriter == nil {
		c.StepWriter = os.Stdout
	}
	c.Filter = c.Filter.normalized()
	return c
}

func (c Config) validate() error {
	if err := c.StartAt.validate(); err != nil {
		return err
	}
	return c.Filter.validate()
}

type StartAt struct {
	Time     time.Time
	Sequence int64
}

func (s StartAt) Active() bool {
	return !s.Time.IsZero() || s.Sequence > 0
}

func (s StartAt) validate() error {
	if !s.Time.IsZero() && s.Sequence > 0 {
		return fmt.Errorf("config error: start-at supports either time or sequence, not both")
	}
	if s.Sequence < 0 {
		return fmt.Errorf("config error: start-at sequence must be non-negative")
	}
	return nil
}

func (s StartAt) Reached(event Event) bool {
	switch {
	case !s.Time.IsZero():
		return !event.Timestamp().Before(s.Time)
	case s.Sequence > 0:
		return event.Sequence() >= s.Sequence
	default:
		return true
	}
}
