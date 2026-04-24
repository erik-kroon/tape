package tape

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

type Hasher struct {
	hash   hash.Hash
	events int
	codecs eventCodecRegistry
}

func NewHasher() *Hasher {
	hasher, err := NewHasherWithCodecs()
	if err != nil {
		panic(err)
	}
	return hasher
}

func NewHasherWithCodecs(codecs ...EventCodec) (*Hasher, error) {
	registry, err := newEventCodecRegistry(codecs)
	if err != nil {
		return nil, err
	}
	return &Hasher{
		hash:   sha256.New(),
		codecs: registry,
	}, nil
}

func (h *Hasher) Middleware() Middleware {
	return func(next EventHandler) EventHandler {
		return func(ctx Context, event Event) error {
			record, err := h.codecs.Marshal(ctx.Index, event)
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
		hasher, err := NewHasherWithCodecs(config.EventCodecs...)
		if err != nil {
			return DeterminismResult{}, err
		}
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
