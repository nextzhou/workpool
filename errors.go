package workpool

import "fmt"

// Recover provides a hook to catch the panic of a task running.
type Recover = func(err ErrPanic) error

// ErrPanic consists of various information about a panic
type ErrPanic struct {
	Recover interface{}
	Stack   []byte
}

func (e ErrPanic) Error() string {
	return fmt.Sprintf("panic: %v\nstack:\n%s", e.Recover, e.Stack)
}

type ErrSkipPendingTask struct {
	SKippingTaskCount uint
}

func (e ErrSkipPendingTask) Error() string {
	return fmt.Sprintf("skip %d pending tasks", e.SKippingTaskCount)
}
