package tape

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

type Hasher struct {
	hash   hash.Hash
	events int
}

func NewHasher() *Hasher {
	return &Hasher{hash: sha256.New()}
}

func (h *Hasher) Middleware() Middleware {
	return func(next EventHandler) EventHandler {
		return func(ctx Context, event Event) error {
			record, err := marshalSessionRecord(ctx.Index, event)
			if err != nil {
				return err
			}
			if _, err := h.hash.Write(record); err != nil {
				return err
			}
			h.events++
			return next(ctx, event)
		}
	}
}

func (h *Hasher) Sum() string {
	return hex.EncodeToString(h.hash.Sum(nil))
}

type DeterminismResult struct {
	Runs   int
	Events int
	Hash   string
}

func CheckDeterminism(path string, config Config, runs int) (DeterminismResult, error) {
	if runs <= 0 {
		runs = 1
	}

	var expectedHash string
	var totalEvents int

	for attempt := 0; attempt < runs; attempt++ {
		engine := NewEngine(config)
		hasher := NewHasher()
		engine.Use(hasher.Middleware())

		summary, err := engine.RunFile(path)
		if err != nil {
			return DeterminismResult{}, err
		}

		sum := hasher.Sum()
		if attempt == 0 {
			expectedHash = sum
			totalEvents = summary.Events
			continue
		}

		if sum != expectedHash {
			return DeterminismResult{}, ErrDeterminismMismatch
		}
	}

	return DeterminismResult{
		Runs:   runs,
		Events: totalEvents,
		Hash:   expectedHash,
	}, nil
}
