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

type eventCodecRegistry struct {
	byType map[string]EventCodec
}

func newEventCodecRegistry(custom []EventCodec) (eventCodecRegistry, error) {
	registry := eventCodecRegistry{
		byType: map[string]EventCodec{},
	}

	for _, codec := range builtinEventCodecs() {
		registry.byType[codec.Type] = codec
	}

	for _, codec := range custom {
		if err := validateEventCodec(codec); err != nil {
			return eventCodecRegistry{}, err
		}
		if _, exists := registry.byType[codec.Type]; exists {
			return eventCodecRegistry{}, fmt.Errorf("codec error: duplicate event codec %q", codec.Type)
		}
		registry.byType[codec.Type] = codec
	}

	return registry, nil
}

func (r eventCodecRegistry) Marshal(index int, event Event) ([]byte, error) {
	codec, ok := r.byType[event.Type()]
	if !ok {
		return nil, fmt.Errorf("sink error: unsupported event type %T", event)
	}

	payload, err := codec.Encode(event)
	if err != nil {
		return nil, err
	}

	return json.Marshal(sessionRecord{
		Type:    codec.Type,
		Payload: payload,
		Index:   index,
	})
}

func (r eventCodecRegistry) Decode(record sessionRecord) (Event, error) {
	codec, ok := r.byType[record.Type]
	if !ok {
		return nil, fmt.Errorf("decode error: unknown event type %q", record.Type)
	}

	payload, err := record.payload()
	if err != nil {
		return nil, err
	}

	event, err := codec.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("decode error: invalid %s payload: %w", record.Type, err)
	}
	return event, nil
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
