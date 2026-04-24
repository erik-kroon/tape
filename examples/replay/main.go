package main

import (
	"fmt"
	"log"

	"github.com/erik-kroon/tape/internal/exampledata"
	tape "github.com/erik-kroon/tape/src"
)

func main() {
	engine := tape.NewEngine(tape.Config{
		Mode:  tape.AcceleratedMode,
		Speed: 100,
	})

	engine.OnBar(func(ctx tape.Context, bar tape.Bar) error {
		fmt.Printf("%s %s close=%.2f\n", ctx.Clock().Now().Format("15:04:05"), bar.Symbol(), bar.Close)
		return nil
	})

	summary, err := engine.RunFile(exampledata.Path("testdata", "bars_5_rows.csv"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Replayed %d bar events across %d symbol(s)\n", summary.Events, len(summary.Symbols))
}
