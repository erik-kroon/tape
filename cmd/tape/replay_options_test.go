package main

import (
	"strings"
	"testing"

	tape "github.com/erik-kroon/tape/src"
)

func TestReplayCommandOptionsParseAndBuildReplayConfig(t *testing.T) {
	options := newReplayCommandOptions()
	err := options.parse(replaySpec, []string{
		"./session.tape",
		"--speed", "25x",
		"--step",
		"--symbol", "ERICB, VOLV",
		"--event-type", "tick",
		"--from", "2026-04-24T09:30:00Z",
		"--to", "2026-04-24T09:31:00Z",
		"--start-at", "4",
		"--record", "./copy.tape",
		"--metrics=false",
		"--print",
		"--permissive",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	config, err := options.config()
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	if got := strings.Join(options.paths, ","); got != "./session.tape" {
		t.Fatalf("paths = %q, want ./session.tape", got)
	}
	if !options.step || !options.print || options.metrics || !options.permissive {
		t.Fatalf("unexpected option flags: %+v", options)
	}
	if options.recordPath != "./copy.tape" {
		t.Fatalf("record path = %q, want ./copy.tape", options.recordPath)
	}
	if config.Mode != tape.StepMode {
		t.Fatalf("mode = %s, want %s", config.Mode, tape.StepMode)
	}
	if config.StartAt.Sequence != 4 {
		t.Fatalf("start-at sequence = %d, want 4", config.StartAt.Sequence)
	}
	if got := strings.Join(config.Filter.Symbols, ","); got != "ERICB,VOLV" {
		t.Fatalf("symbols = %q, want ERICB,VOLV", got)
	}
	if got := strings.Join(config.Filter.EventTypes, ","); got != "tick" {
		t.Fatalf("event types = %q, want tick", got)
	}
}

func TestReplayCommandOptionsBuildInspectConfigDefaultsToMaxSpeed(t *testing.T) {
	options := newReplayCommandOptions()
	err := options.parse(inspectSpec, []string{
		"--start-at", "2026-04-24",
		"--sample", "2",
		"./session.tape",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	config, err := options.config()
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	if config.Mode != tape.MaxSpeedMode {
		t.Fatalf("mode = %s, want %s", config.Mode, tape.MaxSpeedMode)
	}
	if config.StartAt.Time.IsZero() {
		t.Fatal("start-at time is zero")
	}
	if options.sample != 2 {
		t.Fatalf("sample = %d, want 2", options.sample)
	}
}

func TestReplayCommandOptionsParseMultiplePaths(t *testing.T) {
	options := newReplayCommandOptions()
	err := options.parse(replaySpec, []string{
		"./quotes.tape",
		"./trades.tape",
		"--metrics=false",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := strings.Join(options.paths, ","); got != "./quotes.tape,./trades.tape" {
		t.Fatalf("paths = %q, want ./quotes.tape,./trades.tape", got)
	}
}

func TestReplayCommandOptionsRejectInvalidUsage(t *testing.T) {
	options := newReplayCommandOptions()
	err := options.parse(checkSpec, []string{"--runs", "2"})
	if err == nil {
		t.Fatal("parse error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), checkUsageLine) {
		t.Fatalf("error = %v, want usage line", err)
	}
}
