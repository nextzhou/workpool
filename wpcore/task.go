package wpcore

import (
	"context"
	"runtime/debug"
)

type Task func(context.Context) error

type TaskWait func() error

func (t Task) Go(ctx context.Context) TaskWait {
	errC := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				errC <- ErrPanic{Recover: r, Stack: stack}
			}
		}()
		err := t(ctx)
		if err == nil {
			errC <- nil
		} else {
			errC <- UniformError{Err: err}
		}
	}()

	//nolint: errorlint,forcetypeassert
	return func() error {
		err := <-errC
		if err == nil {
			return nil
		}
		if err, ok := err.(UniformError); ok {
			return err.Err
		}
		panic(err.(ErrPanic))
	}
}

type TaskWrap func(Task) Task

func PanicAsError(t Task) Task {
	return func(ctx context.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = ErrPanic{
					Recover: r,
					Stack:   debug.Stack(),
				}
			}
		}()
		err = t(ctx)
		return
	}
}

func RunStopTask(run, stop func() error) Task {
	var task Task = func(ctx context.Context) error {
		return run()
	}
	return func(ctx context.Context) error {
		wait := task.Go(ctx)

		<-ctx.Done()
		if err := stop(); err != nil {
			return err
		}
		return wait()
	}
}
