package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/erik-kroon/tape"
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
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, map[string]bool{
		"--speed":  true,
		"--record": true,
	})); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: tape replay <path> [--speed max|realtime|100x] [--step]")
	}

	config, err := configFromFlags(*speed, *step, *permissive)
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
	permissive := flags.Bool("permissive", false, "continue through out-of-order timestamps")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, nil)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: tape inspect <path>")
	}

	engine := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode, Permissive: *permissive})
	summary, err := engine.RunFile(flags.Arg(0))
	if err != nil {
		return err
	}

	fmt.Printf("Events:      %d\n", summary.Events)
	fmt.Printf("First event: %s\n", formatTimestamp(summary.FirstEventTime))
	fmt.Printf("Last event:  %s\n", formatTimestamp(summary.LastEventTime))
	fmt.Printf("Types:       %s\n", formatCountMap(summary.EventTypes))
	fmt.Printf("Symbols:     %s\n", formatCountMap(summary.Symbols))
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
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, map[string]bool{
		"--runs": true,
	})); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: tape check <path> [--runs N]")
	}

	result, err := tape.CheckDeterminism(flags.Arg(0), tape.Config{
		Mode:       tape.MaxSpeedMode,
		Permissive: *permissive,
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

func configFromFlags(speed string, step bool, permissive bool) (tape.Config, error) {
	if step {
		return tape.Config{Mode: tape.StepMode, Permissive: permissive}, nil
	}

	normalized := strings.ToLower(strings.TrimSpace(speed))
	switch normalized {
	case "", "max":
		return tape.Config{Mode: tape.MaxSpeedMode, Permissive: permissive}, nil
	case "realtime", "real-time", "1x":
		return tape.Config{Mode: tape.RealTimeMode, Speed: 1, Permissive: permissive}, nil
	default:
		if !strings.HasSuffix(normalized, "x") {
			return tape.Config{}, fmt.Errorf("config error: invalid speed %q", speed)
		}
		value, err := strconv.ParseFloat(strings.TrimSuffix(normalized, "x"), 64)
		if err != nil || value <= 0 {
			return tape.Config{}, fmt.Errorf("config error: invalid speed %q", speed)
		}
		return tape.Config{Mode: tape.AcceleratedMode, Speed: value, Permissive: permissive}, nil
	}
}

func printSummary(summary tape.Summary) {
	fmt.Println("Replay completed.")
	fmt.Printf("Events:       %d\n", summary.Events)
	fmt.Printf("First event:  %s\n", formatTimestamp(summary.FirstEventTime))
	fmt.Printf("Last event:   %s\n", formatTimestamp(summary.LastEventTime))
	fmt.Printf("Elapsed:      %s\n", summary.WallDuration.Round(time.Microsecond))
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
	fmt.Println("  tape replay <path> [--speed max|realtime|100x] [--step]")
	fmt.Println("  tape inspect <path>")
	fmt.Println("  tape bench [--events N] [--symbols N]")
	fmt.Println("  tape check <path> [--runs N]")
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
