package workpool

import "fmt"

type Recover = func(err ErrPanic) error

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
