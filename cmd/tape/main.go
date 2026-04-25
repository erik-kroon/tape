package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
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
	case "index":
		return runIndex(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runReplay(args []string) error {
	options := newReplayCommandOptions()
	if err := options.parse(replaySpec, args); err != nil {
		return err
	}

	config, err := options.config()
	if err != nil {
		return err
	}

	engine := tape.NewEngine(config)
	if options.print || options.step {
		engine.AddSink(tape.OutputSinkFunc(func(ctx tape.Context, event tape.Event) error {
			fmt.Println(formatEvent(ctx.Index, event))
			return nil
		}))
	}

	var recorder *tape.Recorder
	if options.recordPath != "" {
		recorder, err = tape.NewRecorder(options.recordPath)
		if err != nil {
			return err
		}
		defer recorder.Close()
		engine.AddSink(recorder)
	}

	summary, err := engine.RunFiles(options.paths...)
	if err != nil {
		return err
	}
	if options.metrics {
		printSummary(summary)
	}
	return nil
}

func runInspect(args []string) error {
	options := newReplayCommandOptions()
	if err := options.parse(inspectSpec, args); err != nil {
		return err
	}

	config, err := options.config()
	if err != nil {
		return err
	}

	engine := tape.NewEngine(config)
	summary, err := engine.RunFiles(options.paths...)
	if err != nil {
		return err
	}
	samples, err := sampleEvents(options.paths, options.sample, config)
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
	options := newReplayCommandOptions()
	if err := options.parse(checkSpec, args); err != nil {
		return err
	}

	config, err := options.config()
	if err != nil {
		return err
	}

	result, err := tape.CheckDeterminismFiles(options.paths, config, options.runs)
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

func runIndex(args []string) error {
	flags := flag.NewFlagSet("index", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(reorderArgs(args, map[string]bool{})); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: tape index <path>")
	}

	if err := tape.BuildSessionIndex(flags.Arg(0)); err != nil {
		return err
	}

	fmt.Printf("Index written: %s\n", flags.Arg(0)+".idx")
	return nil
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
	fmt.Println("  " + replayUsageLine)
	fmt.Println("  " + inspectUsageLine)
	fmt.Println("  tape bench [--events N] [--symbols N]")
	fmt.Println("  " + checkUsageLine)
	fmt.Println("  tape index <path>")
}

func sampleEvents(paths []string, limit int, config tape.Config) ([]tape.Event, error) {
	if limit <= 0 {
		return nil, nil
	}

	selection, err := tape.OpenReplaySelectionPaths(paths, config)
	if err != nil {
		return nil, err
	}
	defer selection.Close()

	samples := make([]tape.Event, 0, limit)
	for len(samples) < limit {
		selected, err := selection.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return samples, nil
			}
			return nil, err
		}
		samples = append(samples, selected.Event)
	}

	return samples, nil
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
