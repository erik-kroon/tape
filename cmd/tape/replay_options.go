package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tape "github.com/erik-kroon/tape/src"
)

const (
	replayUsageLine  = "tape replay <path> [--speed max|realtime|100x] [--step] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]"
	inspectUsageLine = "tape inspect <path> [--sample N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]"
	checkUsageLine   = "tape check <path> [--runs N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]"
)

type replayCommandSpec struct {
	name        string
	usage       string
	withSpeed   bool
	withStep    bool
	withPrint   bool
	withMetrics bool
	withRecord  bool
	withSample  bool
	withRuns    bool
}

type replayCommandOptions struct {
	path       string
	speed      string
	step       bool
	print      bool
	metrics    bool
	recordPath string
	sample     int
	runs       int
	permissive bool
	symbols    string
	eventTypes string
	from       string
	to         string
	startAt    string
}

var (
	replaySpec = replayCommandSpec{
		name:        "replay",
		usage:       replayUsageLine,
		withSpeed:   true,
		withStep:    true,
		withPrint:   true,
		withMetrics: true,
		withRecord:  true,
	}
	inspectSpec = replayCommandSpec{
		name:       "inspect",
		usage:      inspectUsageLine,
		withSample: true,
	}
	checkSpec = replayCommandSpec{
		name:     "check",
		usage:    checkUsageLine,
		withRuns: true,
	}
)

func newReplayCommandOptions() replayCommandOptions {
	return replayCommandOptions{
		speed:   "max",
		metrics: true,
		sample:  3,
		runs:    5,
	}
}

func (o *replayCommandOptions) parse(spec replayCommandSpec, args []string) error {
	flags, valueFlags := o.newFlagSet(spec)
	if err := flags.Parse(reorderArgs(args, valueFlags)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: " + spec.usage)
	}

	o.path = flags.Arg(0)
	return nil
}

func (o *replayCommandOptions) newFlagSet(spec replayCommandSpec) (*flag.FlagSet, map[string]bool) {
	flags := flag.NewFlagSet(spec.name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	valueFlags := map[string]bool{}

	stringFlag := func(name string, target *string, value string, usage string) {
		flags.StringVar(target, name, value, usage)
		valueFlags["--"+name] = true
	}
	intFlag := func(name string, target *int, value int, usage string) {
		flags.IntVar(target, name, value, usage)
		valueFlags["--"+name] = true
	}

	if spec.withSpeed {
		stringFlag("speed", &o.speed, o.speed, "replay speed: max, realtime, or <Nx>")
	}
	if spec.withStep {
		flags.BoolVar(&o.step, "step", false, "step through one event at a time")
	}
	if spec.withPrint {
		flags.BoolVar(&o.print, "print", false, "print each event as it is replayed")
	}
	if spec.withMetrics {
		flags.BoolVar(&o.metrics, "metrics", o.metrics, "print replay summary")
	}
	if spec.withRecord {
		stringFlag("record", &o.recordPath, "", "record events to a .tape file")
	}
	if spec.withSample {
		intFlag("sample", &o.sample, o.sample, "number of sample events to print")
	}
	if spec.withRuns {
		intFlag("runs", &o.runs, o.runs, "number of replay runs")
	}

	flags.BoolVar(&o.permissive, "permissive", false, "continue through out-of-order timestamps")
	stringFlag("symbol", &o.symbols, "", "comma-separated symbol filters")
	stringFlag("event-type", &o.eventTypes, "", "comma-separated event-type filters")
	stringFlag("from", &o.from, "", "inclusive replay start time (RFC3339)")
	stringFlag("to", &o.to, "", "inclusive replay end time (RFC3339)")
	stringFlag("start-at", &o.startAt, "", "seek to the first event at or after an RFC3339 timestamp, date, or sequence")

	return flags, valueFlags
}

func (o replayCommandOptions) config() (tape.Config, error) {
	filter, err := filterFromFlags(o.symbols, o.eventTypes, o.from, o.to)
	if err != nil {
		return tape.Config{}, err
	}

	position, err := startAtFromFlag(o.startAt)
	if err != nil {
		return tape.Config{}, err
	}

	return configFromFlags(o.speed, o.step, o.permissive, filter, position)
}

func configFromFlags(speed string, step bool, permissive bool, filter tape.Filter, startAt tape.StartAt) (tape.Config, error) {
	if step {
		return tape.Config{Mode: tape.StepMode, Permissive: permissive, Filter: filter, StartAt: startAt}, nil
	}

	normalized := strings.ToLower(strings.TrimSpace(speed))
	switch normalized {
	case "", "max":
		return tape.Config{Mode: tape.MaxSpeedMode, Permissive: permissive, Filter: filter, StartAt: startAt}, nil
	case "realtime", "real-time", "1x":
		return tape.Config{Mode: tape.RealTimeMode, Speed: 1, Permissive: permissive, Filter: filter, StartAt: startAt}, nil
	default:
		if !strings.HasSuffix(normalized, "x") {
			return tape.Config{}, fmt.Errorf("config error: invalid speed %q", speed)
		}
		value, err := strconv.ParseFloat(strings.TrimSuffix(normalized, "x"), 64)
		if err != nil || value <= 0 {
			return tape.Config{}, fmt.Errorf("config error: invalid speed %q", speed)
		}
		return tape.Config{Mode: tape.AcceleratedMode, Speed: value, Permissive: permissive, Filter: filter, StartAt: startAt}, nil
	}
}

func filterFromFlags(symbols string, eventTypes string, from string, to string) (tape.Filter, error) {
	startTime, err := parseFilterTime("from", from)
	if err != nil {
		return tape.Filter{}, err
	}
	endTime, err := parseFilterTime("to", to)
	if err != nil {
		return tape.Filter{}, err
	}

	return tape.Filter{
		Symbols:    splitFilterValues(symbols),
		EventTypes: splitFilterValues(eventTypes),
		StartTime:  startTime,
		EndTime:    endTime,
	}, nil
}

func splitFilterValues(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		values = append(values, value)
	}

	if len(values) == 0 {
		return nil
	}
	return values
}

func parseFilterTime(name string, raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("config error: invalid %s time %q", name, raw)
}

func startAtFromFlag(raw string) (tape.StartAt, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return tape.StartAt{}, nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return tape.StartAt{Time: parsed}, nil
		}
	}

	sequence, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		if sequence < 0 {
			return tape.StartAt{}, fmt.Errorf("config error: start-at sequence must be non-negative")
		}
		return tape.StartAt{Sequence: sequence}, nil
	}

	return tape.StartAt{}, fmt.Errorf("config error: invalid start-at %q", raw)
}
