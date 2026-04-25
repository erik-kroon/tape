package strategy

import (
	"errors"

	tape "github.com/erik-kroon/tape/src"
)

type RunContext struct {
	Path string
}

type RunResult struct {
	Path    string
	Summary tape.Summary
	Err     error
}

type Hooks struct {
	OnStart func(RunContext) error
	OnEvent func(tape.Context, tape.Event) error
	OnEnd   func(RunResult) error
}

type Runner struct {
	config     tape.Config
	middleware []tape.Middleware
	sinks      []tape.OutputSink
}

type Option func(*Runner)

func NewRunner(config tape.Config, options ...Option) Runner {
	runner := Runner{config: config}
	for _, option := range options {
		option(&runner)
	}
	return runner
}

func RunFile(path string, config tape.Config, hooks Hooks, options ...Option) (tape.Summary, error) {
	return NewRunner(config, options...).RunFile(path, hooks)
}

func Run(stream tape.Stream, config tape.Config, hooks Hooks, options ...Option) (tape.Summary, error) {
	return NewRunner(config, options...).Run(stream, hooks)
}

func WithMiddleware(middleware ...tape.Middleware) Option {
	return func(runner *Runner) {
		runner.middleware = append(runner.middleware, middleware...)
	}
}

func WithSinks(sinks ...tape.OutputSink) Option {
	return func(runner *Runner) {
		runner.sinks = append(runner.sinks, sinks...)
	}
}

func (r Runner) RunFile(path string, hooks Hooks) (tape.Summary, error) {
	return r.run(RunContext{Path: path}, hooks, func(engine *tape.Engine) (tape.Summary, error) {
		return engine.RunFile(path)
	})
}

func (r Runner) Run(stream tape.Stream, hooks Hooks) (tape.Summary, error) {
	return r.run(RunContext{}, hooks, func(engine *tape.Engine) (tape.Summary, error) {
		return engine.Run(stream)
	})
}

func (r Runner) run(runContext RunContext, hooks Hooks, execute func(*tape.Engine) (tape.Summary, error)) (summary tape.Summary, err error) {
	if hooks.OnEvent == nil {
		err = errors.New("strategy: OnEvent hook is required")
		return summary, finalize(hooks.OnEnd, RunResult{
			Path:    runContext.Path,
			Summary: summary,
			Err:     err,
		})
	}

	defer func() {
		err = finalize(hooks.OnEnd, RunResult{
			Path:    runContext.Path,
			Summary: summary,
			Err:     err,
		})
	}()

	if hooks.OnStart != nil {
		if err = hooks.OnStart(runContext); err != nil {
			return summary, err
		}
	}

	engine := tape.NewEngine(r.config)
	for _, middleware := range r.middleware {
		engine.Use(middleware)
	}
	for _, sink := range r.sinks {
		engine.AddSink(sink)
	}
	engine.OnEvent(hooks.OnEvent)

	return execute(engine)
}

func finalize(onEnd func(RunResult) error, result RunResult) error {
	if onEnd == nil {
		return result.Err
	}

	endErr := onEnd(result)
	if endErr == nil {
		return result.Err
	}
	if result.Err == nil {
		return endErr
	}

	return errors.Join(result.Err, endErr)
}
