package strategy

import (
	"errors"

	tape "github.com/erik-kroon/tape/src"
)

type RunContext struct {
	Path  string
	Paths []string
}

type RunResult struct {
	Path    string
	Paths   []string
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
	ranges     ReplayRanges
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
	return RunFiles([]string{path}, config, hooks, options...)
}

func RunFiles(paths []string, config tape.Config, hooks Hooks, options ...Option) (tape.Summary, error) {
	return NewRunner(config, options...).RunFiles(paths, hooks)
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

func WithReplayRanges(ranges ReplayRanges) Option {
	return func(runner *Runner) {
		runner.ranges = ranges
	}
}

func (r Runner) RunFile(path string, hooks Hooks) (tape.Summary, error) {
	return r.RunFiles([]string{path}, hooks)
}

func (r Runner) RunFiles(paths []string, hooks Hooks) (tape.Summary, error) {
	runContext := newRunContext(paths)
	return r.run(runContext, hooks, func(engine *tape.Engine) (tape.Summary, error) {
		return engine.RunFiles(paths...)
	})
}

func (r Runner) Run(stream tape.Stream, hooks Hooks) (tape.Summary, error) {
	return r.run(RunContext{}, hooks, func(engine *tape.Engine) (tape.Summary, error) {
		return engine.Run(stream)
	})
}

func newRunContext(paths []string) RunContext {
	cloned := append([]string(nil), paths...)
	context := RunContext{Paths: cloned}
	if len(cloned) == 1 {
		context.Path = cloned[0]
	}
	return context
}

func newRunResult(runContext RunContext, summary tape.Summary, err error) RunResult {
	return RunResult{
		Path:    runContext.Path,
		Paths:   append([]string(nil), runContext.Paths...),
		Summary: summary,
		Err:     err,
	}
}

func (r Runner) run(runContext RunContext, hooks Hooks, execute func(*tape.Engine) (tape.Summary, error)) (summary tape.Summary, err error) {
	if hooks.OnEvent == nil {
		err = errors.New("strategy: OnEvent hook is required")
		return summary, finalize(hooks.OnEnd, newRunResult(runContext, summary, err))
	}

	config, err := r.runtimeConfig()
	if err != nil {
		return summary, finalize(hooks.OnEnd, newRunResult(runContext, summary, err))
	}

	defer func() {
		err = finalize(hooks.OnEnd, newRunResult(runContext, summary, err))
	}()

	if hooks.OnStart != nil {
		if err = hooks.OnStart(runContext); err != nil {
			return summary, err
		}
	}

	engine := tape.NewEngine(config)
	for _, middleware := range r.middleware {
		engine.Use(middleware)
	}
	for _, sink := range r.sinks {
		engine.AddSink(sink)
	}
	engine.OnEvent(r.wrapOnEvent(hooks.OnEvent))

	return execute(engine)
}

func (r Runner) runtimeConfig() (tape.Config, error) {
	config := r.config
	filter, err := r.ranges.combinedFilter(config.Filter)
	if err != nil {
		return tape.Config{}, err
	}
	config.Filter = filter
	return config, nil
}

func (r Runner) wrapOnEvent(onEvent func(tape.Context, tape.Event) error) func(tape.Context, tape.Event) error {
	measuredIndex := 0
	return func(ctx tape.Context, event tape.Event) error {
		return onEvent(r.ranges.decorateContext(ctx, event, &measuredIndex), event)
	}
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
