package main

import (
	"fmt"
	"log"

	"github.com/erik-kroon/tape/internal/exampledata"
	tape "github.com/erik-kroon/tape/src"
	"github.com/erik-kroon/tape/strategy"
)

func main() {
	summary, err := strategy.RunFile(exampledata.Path("testdata", "bars_5_rows.csv"), tape.Config{
		Mode: tape.MaxSpeedMode,
	}, strategy.Hooks{
		OnStart: func(ctx strategy.RunContext) error {
			fmt.Printf("Starting strategy on %s\n", ctx.Path)
			return nil
		},
		OnEvent: func(ctx tape.Context, event tape.Event) error {
			bar, ok := event.(tape.Bar)
			if !ok {
				return nil
			}
			fmt.Printf("bar[%d] %s close=%.2f\n", ctx.Index, bar.Symbol(), bar.Close)
			return nil
		},
		OnEnd: func(result strategy.RunResult) error {
			fmt.Printf("Strategy completed with %d events\n", result.Summary.Events)
			return nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Harness summary events=%d symbols=%d\n", summary.Events, len(summary.Symbols))
}
