package tape

import (
	"errors"
	"fmt"
)

var ErrDeterminismMismatch = errors.New("determinism check failed")
var ErrHandlerPanic = errors.New("handler panic")

type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	if e == nil {
		return ErrHandlerPanic.Error()
	}
	return fmt.Sprintf("%s: %v", ErrHandlerPanic, e.Value)
}

func (e *PanicError) Unwrap() error {
	return ErrHandlerPanic
}
