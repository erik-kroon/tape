package tape

import (
	"fmt"
	"math/rand"
	"time"
)

type BenchmarkResult struct {
	Events     int
	Symbols    int
	Elapsed    time.Duration
	Throughput float64
}

func RunSyntheticBenchmark(config Config, events int, symbols int, seed int64) (BenchmarkResult, error) {
	if events <= 0 {
		return BenchmarkResult{}, fmt.Errorf("config error: events must be positive")
	}
	if symbols <= 0 {
		return BenchmarkResult{}, fmt.Errorf("config error: symbols must be positive")
	}

	generator := rand.New(rand.NewSource(seed))
	symbolList := make([]string, symbols)
	for index := 0; index < symbols; index++ {
		symbolList[index] = fmt.Sprintf("SYM%03d", index+1)
	}

	start := time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC)
	generated := make([]Event, 0, events)
	for index := 0; index < events; index++ {
		generated = append(generated, Tick{
			Time:  start.Add(time.Duration(index) * time.Microsecond),
			Sym:   symbolList[index%symbols],
			Price: 100 + generator.Float64()*10,
			Size:  float64(1 + generator.Intn(50)),
			Seq:   int64(index + 1),
		})
	}

	engine := NewEngine(config)
	engine.OnEvent(func(ctx Context, event Event) error {
		return nil
	})

	summary, err := engine.Run(newSliceStream(generated))
	if err != nil {
		return BenchmarkResult{}, err
	}

	throughput := float64(summary.Events)
	if summary.WallDuration > 0 {
		throughput = float64(summary.Events) / summary.WallDuration.Seconds()
	}

	return BenchmarkResult{
		Events:     summary.Events,
		Symbols:    symbols,
		Elapsed:    summary.WallDuration,
		Throughput: throughput,
	}, nil
}
