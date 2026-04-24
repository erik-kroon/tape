package tape

import (
	"runtime/debug"
	"time"
)

func RecoverPanics() Middleware {
	return func(next EventHandler) EventHandler {
		return func(ctx Context, event Event) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = &PanicError{
						Value: recovered,
						Stack: debug.Stack(),
					}
				}
			}()

			return next(ctx, event)
		}
	}
}

type HandlerTimer struct {
	total time.Duration
}

func NewHandlerTimer() *HandlerTimer {
	return &HandlerTimer{}
}

func (t *HandlerTimer) Middleware() Middleware {
	return func(next EventHandler) EventHandler {
		return func(ctx Context, event Event) error {
			startedAt := time.Now()
			err := next(ctx, event)
			t.total += time.Since(startedAt)
			return err
		}
	}
}

func (t *HandlerTimer) Total() time.Duration {
	if t == nil {
		return 0
	}
	return t.total
}
