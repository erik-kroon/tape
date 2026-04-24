package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tape "github.com/erik-kroon/tape/src"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "replay":
		return runReplay(args[1:])
	case "inspect":
		return runInspect(args[1:])
	case "bench":
		return runBench(args[1:])
	case "check":
		return runCheck(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runReplay(args []string) error {
	flags := flag.NewFlagSet("replay", flag.ContinueOnError)
	speed := flags.String("speed", "max", "replay speed: max, realtime, or <Nx>")
	step := flags.Bool("step", false, "step through one event at a time")
	printEvents := flags.Bool("print", false, "print each event as it is replayed")
	metrics := flags.Bool("metrics", true, "print replay summary")
	recordPath := flags.String("record", "", "record events to a .tape file")
	permissive := flags.Bool("permissive", false, "continue through out-of-order timestamps")
	symbols := flags.String("symbol", "", "comma-separated symbol filters")
	eventTypes := flags.String("event-type", "", "comma-separated event-type filters")
	from := flags.String("from", "", "inclusive replay start time (RFC3339)")
	to := flags.String("to", "", "inclusive replay end time (RFC3339)")
	startAt := flags.String("start-at", "", "seek to the first event at or after an RFC3339 timestamp, date, or sequence")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, map[string]bool{
		"--speed":      true,
		"--record":     true,
		"--symbol":     true,
		"--event-type": true,
		"--from":       true,
		"--to":         true,
		"--start-at":   true,
	})); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: tape replay <path> [--speed max|realtime|100x] [--step] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]")
	}

	filter, err := filterFromFlags(*symbols, *eventTypes, *from, *to)
	if err != nil {
		return err
	}
	position, err := startAtFromFlag(*startAt)
	if err != nil {
		return err
	}

	config, err := configFromFlags(*speed, *step, *permissive, filter, position)
	if err != nil {
		return err
	}

	engine := tape.NewEngine(config)
	if *printEvents || *step {
		engine.OnEvent(func(ctx tape.Context, event tape.Event) error {
			fmt.Println(formatEvent(ctx.Index, event))
			return nil
		})
	}

	var recorder *tape.Recorder
	if *recordPath != "" {
		recorder, err = tape.NewRecorder(*recordPath)
		if err != nil {
			return err
		}
		defer recorder.Close()
		engine.Use(recorder.Middleware())
	}

	summary, err := engine.RunFile(flags.Arg(0))
	if err != nil {
		return err
	}
	if *metrics {
		printSummary(summary)
	}
	return nil
}

func runInspect(args []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	sample := flags.Int("sample", 3, "number of sample events to print")
	permissive := flags.Bool("permissive", false, "continue through out-of-order timestamps")
	symbols := flags.String("symbol", "", "comma-separated symbol filters")
	eventTypes := flags.String("event-type", "", "comma-separated event-type filters")
	from := flags.String("from", "", "inclusive replay start time (RFC3339)")
	to := flags.String("to", "", "inclusive replay end time (RFC3339)")
	startAt := flags.String("start-at", "", "seek to the first event at or after an RFC3339 timestamp, date, or sequence")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, map[string]bool{
		"--sample":     true,
		"--symbol":     true,
		"--event-type": true,
		"--from":       true,
		"--to":         true,
		"--start-at":   true,
	})); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: tape inspect <path> [--sample N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]")
	}

	filter, err := filterFromFlags(*symbols, *eventTypes, *from, *to)
	if err != nil {
		return err
	}
	position, err := startAtFromFlag(*startAt)
	if err != nil {
		return err
	}

	path := flags.Arg(0)
	config := tape.Config{
		Mode:       tape.MaxSpeedMode,
		Permissive: *permissive,
		Filter:     filter,
		StartAt:    position,
	}
	engine := tape.NewEngine(config)
	summary, err := engine.RunFile(path)
	if err != nil {
		return err
	}
	samples, err := sampleEvents(path, *sample, config)
	if err != nil {
		return err
	}

	fmt.Printf("Events:      %d\n", summary.Events)
	fmt.Printf("First event: %s\n", formatTimestamp(summary.FirstEventTime))
	fmt.Printf("Last event:  %s\n", formatTimestamp(summary.LastEventTime))
	fmt.Printf("Types:       %s\n", formatCountMap(summary.EventTypes))
	fmt.Printf("Symbols:     %s\n", formatCountMap(summary.Symbols))
	if len(samples) > 0 {
		fmt.Println("Sample:")
		for index, event := range samples {
			fmt.Printf("  %s\n", formatEvent(index, event))
		}
	}
	return nil
}

func runBench(args []string) error {
	flags := flag.NewFlagSet("bench", flag.ContinueOnError)
	events := flags.Int("events", 1000000, "number of synthetic events")
	symbols := flags.Int("symbols", 100, "number of synthetic symbols")
	seed := flags.Int64("seed", 42, "random seed")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, map[string]bool{
		"--events":  true,
		"--symbols": true,
		"--seed":    true,
	})); err != nil {
		return err
	}

	result, err := tape.RunSyntheticBenchmark(tape.Config{Mode: tape.MaxSpeedMode}, *events, *symbols, *seed)
	if err != nil {
		return err
	}

	fmt.Println("Synthetic benchmark completed.")
	fmt.Printf("Events:     %d\n", result.Events)
	fmt.Printf("Symbols:    %d\n", result.Symbols)
	fmt.Printf("Elapsed:    %s\n", result.Elapsed.Round(time.Microsecond))
	fmt.Printf("Throughput: %.0f events/sec\n", result.Throughput)
	return nil
}

func runCheck(args []string) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	runs := flags.Int("runs", 5, "number of replay runs")
	permissive := flags.Bool("permissive", false, "continue through out-of-order timestamps")
	symbols := flags.String("symbol", "", "comma-separated symbol filters")
	eventTypes := flags.String("event-type", "", "comma-separated event-type filters")
	from := flags.String("from", "", "inclusive replay start time (RFC3339)")
	to := flags.String("to", "", "inclusive replay end time (RFC3339)")
	startAt := flags.String("start-at", "", "seek to the first event at or after an RFC3339 timestamp, date, or sequence")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, map[string]bool{
		"--runs":       true,
		"--symbol":     true,
		"--event-type": true,
		"--from":       true,
		"--to":         true,
		"--start-at":   true,
	})); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: tape check <path> [--runs N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]")
	}

	filter, err := filterFromFlags(*symbols, *eventTypes, *from, *to)
	if err != nil {
		return err
	}
	position, err := startAtFromFlag(*startAt)
	if err != nil {
		return err
	}

	result, err := tape.CheckDeterminism(flags.Arg(0), tape.Config{
		Mode:       tape.MaxSpeedMode,
		Permissive: *permissive,
		Filter:     filter,
		StartAt:    position,
	}, *runs)
	if err != nil {
		return err
	}

	fmt.Println("Determinism check passed.")
	fmt.Println()
	fmt.Printf("Runs:             %d\n", result.Runs)
	fmt.Printf("Events processed: %d\n", result.Events)
	fmt.Printf("Output hash:      %s\n", result.Hash[:8])
	return nil
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

func printSummary(summary tape.Summary) {
	fmt.Println("Replay completed.")
	fmt.Printf("Events:       %d\n", summary.Events)
	fmt.Printf("First event:  %s\n", formatTimestamp(summary.FirstEventTime))
	fmt.Printf("Last event:   %s\n", formatTimestamp(summary.LastEventTime))
	fmt.Printf("Elapsed:      %s\n", summary.WallDuration.Round(time.Microsecond))
	fmt.Printf("Handler time: %s\n", summary.HandlerDuration.Round(time.Microsecond))
	fmt.Printf("Throughput:   %.0f events/sec\n", summary.Throughput)
	fmt.Printf("Allocations:  %s\n", formatBytes(summary.AllocBytes))
	fmt.Printf("Errors:       %d\n", summary.ErrorCount)
	fmt.Printf("Types:        %s\n", formatCountMap(summary.EventTypes))
	fmt.Printf("Symbols:      %s\n", formatCountMap(summary.Symbols))
}

func formatEvent(index int, event tape.Event) string {
	base := fmt.Sprintf("seq=%d time=%s symbol=%s", index+1, event.Timestamp().Format(time.RFC3339Nano), event.Symbol())
	switch typed := event.(type) {
	case tape.Tick:
		return fmt.Sprintf("%s price=%.4f size=%.4f", base, typed.Price, typed.Size)
	case tape.Bar:
		return fmt.Sprintf("%s open=%.4f high=%.4f low=%.4f close=%.4f volume=%.4f", base, typed.Open, typed.High, typed.Low, typed.Close, typed.Volume)
	default:
		return base
	}
}

func formatCountMap(counts map[string]int) string {
	if len(counts) == 0 {
		return "-"
	}

	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

func formatTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Format(time.RFC3339Nano)
}

func printUsage() {
	fmt.Println("Tape: deterministic market replay for Go")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  tape replay <path> [--speed max|realtime|100x] [--step] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]")
	fmt.Println("  tape inspect <path> [--sample N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]")
	fmt.Println("  tape bench [--events N] [--symbols N]")
	fmt.Println("  tape check <path> [--runs N] [--symbol SYM1,SYM2] [--event-type tick,bar] [--from RFC3339] [--to RFC3339] [--start-at RFC3339|YYYY-MM-DD|SEQ]")
}

func sampleEvents(path string, limit int, config tape.Config) ([]tape.Event, error) {
	if limit <= 0 {
		return nil, nil
	}

	stream, err := tape.OpenStream(path)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	samples := make([]tape.Event, 0, limit)
	started := !config.StartAt.Active()
	for len(samples) < limit {
		event, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return samples, nil
			}
			return nil, err
		}
		if !started {
			if !config.StartAt.Reached(event) {
				continue
			}
			started = true
		}
		if !config.Filter.Matches(event) {
			continue
		}
		samples = append(samples, event)
	}

	return samples, nil
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

func formatBytes(total uint64) string {
	const unit = 1024
	if total < unit {
		return fmt.Sprintf("%d B", total)
	}

	div := uint64(unit)
	exp := 0
	for value := total / unit; value >= unit; value /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(total)/float64(div), "KMGTPE"[exp])
}

func reorderArgs(args []string, valueFlags map[string]bool) []string {
	var flagArgs []string
	var positional []string

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if strings.Contains(arg, "=") {
				continue
			}
			if valueFlags[arg] && index+1 < len(args) {
				index++
				flagArgs = append(flagArgs, args[index])
			}
			continue
		}
		positional = append(positional, arg)
	}

	return append(flagArgs, positional...)
}
