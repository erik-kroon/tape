package tape

import (
	"encoding/json"
	"fmt"
)

type EventCodec struct {
	Type   string
	Encode func(Event) (json.RawMessage, error)
	Decode func(json.RawMessage) (Event, error)
}

func builtinEventCodecs() []EventCodec {
	return []EventCodec{
		{
			Type: "tick",
			Encode: func(event Event) (json.RawMessage, error) {
				tick, ok := event.(Tick)
				if !ok {
					return nil, fmt.Errorf("sink error: tick codec expected %T to be %T", event, Tick{})
				}
				return json.Marshal(tick)
			},
			Decode: func(payload json.RawMessage) (Event, error) {
				var tick Tick
				if err := json.Unmarshal(payload, &tick); err != nil {
					return nil, err
				}
				return tick, nil
			},
		},
		{
			Type: "bar",
			Encode: func(event Event) (json.RawMessage, error) {
				bar, ok := event.(Bar)
				if !ok {
					return nil, fmt.Errorf("sink error: bar codec expected %T to be %T", event, Bar{})
				}
				return json.Marshal(bar)
			},
			Decode: func(payload json.RawMessage) (Event, error) {
				var bar Bar
				if err := json.Unmarshal(payload, &bar); err != nil {
					return nil, err
				}
				return bar, nil
			},
		},
	}
}

func validateEventCodec(codec EventCodec) error {
	if codec.Type == "" {
		return fmt.Errorf("codec error: event codec type is required")
	}
	if codec.Encode == nil {
		return fmt.Errorf("codec error: event codec %q is missing Encode", codec.Type)
	}
	if codec.Decode == nil {
		return fmt.Errorf("codec error: event codec %q is missing Decode", codec.Type)
	}
	return nil
}
