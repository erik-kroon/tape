package tape

import "errors"

var ErrDeterminismMismatch = errors.New("determinism check failed")
var ErrHandlerPanic = errors.New("handler panic")
