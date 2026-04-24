package main

import (
	"fmt"
	"log"

	tape "github.com/erik-kroon/tape/src"
)

func main() {
	result, err := tape.RunSyntheticBenchmark(tape.Config{Mode: tape.MaxSpeedMode}, 50000, 8, 42)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Benchmark events: %d\n", result.Events)
	fmt.Printf("Benchmark symbols: %d\n", result.Symbols)
	fmt.Printf("Elapsed: %s\n", result.Elapsed.Round(0))
	fmt.Printf("Throughput: %.0f events/sec\n", result.Throughput)
}
