package tape

import "time"

type Event interface {
	Type() string
	Symbol() string
	Timestamp() time.Time
	Sequence() int64
}

type Tick struct {
	Time  time.Time `json:"time"`
	Sym   string    `json:"symbol"`
	Price float64   `json:"price"`
	Size  float64   `json:"size,omitempty"`
	Seq   int64     `json:"seq,omitempty"`
}

func (t Tick) Type() string         { return "tick" }
func (t Tick) Symbol() string       { return t.Sym }
func (t Tick) Timestamp() time.Time { return t.Time }
func (t Tick) Sequence() int64      { return t.Seq }

type Bar struct {
	Time   time.Time `json:"time"`
	Sym    string    `json:"symbol"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume,omitempty"`
	Seq    int64     `json:"seq,omitempty"`
}

func (b Bar) Type() string         { return "bar" }
func (b Bar) Symbol() string       { return b.Sym }
func (b Bar) Timestamp() time.Time { return b.Time }
func (b Bar) Sequence() int64      { return b.Seq }

type sessionRecord struct {
	Type  string `json:"type"`
	Tick  *Tick  `json:"tick,omitempty"`
	Bar   *Bar   `json:"bar,omitempty"`
	Index int    `json:"index,omitempty"`
}
