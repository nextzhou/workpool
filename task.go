package workpool

import (
	"context"
	"runtime/debug"
)

type Task func(context.Context) error

type TaskWrapper func(Task) Task

var _ TaskWrapper = PanicAsError

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
