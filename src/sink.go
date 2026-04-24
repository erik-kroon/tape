package tape

type OutputSink interface {
	Write(Context, Event) error
}

type OutputSinkFunc func(Context, Event) error

func (f OutputSinkFunc) Write(ctx Context, event Event) error {
	return f(ctx, event)
}
