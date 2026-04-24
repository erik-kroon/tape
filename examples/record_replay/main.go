package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/erik-kroon/tape/internal/exampledata"
	tape "github.com/erik-kroon/tape/src"
)

func main() {
	recordingDir, err := os.MkdirTemp("", "tape-record-replay-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(recordingDir)

	recordingPath := filepath.Join(recordingDir, "opening-bell.tape")
	recorder, err := tape.NewRecorder(recordingPath)
	if err != nil {
		log.Fatal(err)
	}

	source := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	source.Use(recorder.Middleware())

	recorded, err := source.RunFile(exampledata.Path("testdata", "ticks_5_rows.csv"))
	closeErr := recorder.Close()
	if err != nil {
		log.Fatal(err)
	}
	if closeErr != nil {
		log.Fatal(closeErr)
	}

	fmt.Printf("Recorded %d tick events into %s\n", recorded.Events, filepath.Base(recordingPath))

	replay := tape.NewEngine(tape.Config{Mode: tape.MaxSpeedMode})
	replay.OnTick(func(ctx tape.Context, tick tape.Tick) error {
		if ctx.Index < 2 {
			fmt.Printf("replay[%d] %s price=%.2f size=%.0f\n", ctx.Index, tick.Symbol(), tick.Price, tick.Size)
		}
		return nil
	})

	replayed, err := replay.RunFile(recordingPath)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Replayed %d events from the recording\n", replayed.Events)
}
