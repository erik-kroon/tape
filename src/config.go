package tape

import (
	"io"
	"os"
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
	return c
}
