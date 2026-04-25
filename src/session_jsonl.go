package tape

import (
	"sync"
)

type Recorder struct {
	mu     sync.Mutex
	writer *sessionFileWriter
}

func NewRecorder(path string) (*Recorder, error) {
	return NewRecorderWithCodecs(path)
}

func NewRecorderWithCodecs(path string, codecs ...EventCodec) (*Recorder, error) {
	session, err := newSessionFile(path, codecs)
	if err != nil {
		return nil, err
	}

	writer, err := session.newWriter()
	if err != nil {
		return nil, err
	}

	return &Recorder{
		writer: writer,
	}, nil
}

func (r *Recorder) Middleware() Middleware {
	return func(next EventHandler) EventHandler {
		return func(ctx Context, event Event) error {
			if err := r.Write(ctx, event); err != nil {
				return err
			}
			return next(ctx, event)
		}
	}
}

func (r *Recorder) Write(ctx Context, event Event) error {
	return r.Record(ctx.Index, event)
}

func (r *Recorder) Record(index int, event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.writer.Write(index, event)
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.writer.Close()
}

func OpenJSONLStream(path string) (Stream, error) {
	return OpenJSONLStreamWithCodecs(path)
}

func OpenJSONLStreamWithCodecs(path string, codecs ...EventCodec) (Stream, error) {
	session, err := newSessionFile(path, codecs)
	if err != nil {
		return nil, err
	}

	return session.openStream()
}
