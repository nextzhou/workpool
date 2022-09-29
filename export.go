package workpool

import (
	"time"

	"github.com/nextzhou/workpool/wpcore"
)

// nolint: gochecknoglobals,nolintlint
var (
	New     = wpcore.New
	Options options
	Wraps   wraps
)

type (
	Task = wpcore.Task
)

// wraps exports task wraps from wpcore.
type wraps struct{}

func (wraps) PanicAsError(task Task) Task {
	return wpcore.PanicAsError(task)
}

func (wraps) Phased(task wpcore.PhasedTask) (Task, wpcore.PhasedTaskSupervisor) {
	return wpcore.Phased(task)
}

func (wraps) RunStopTask(run, stop func() error) Task {
	return wpcore.RunStopTask(run, stop)
}

// options exports options from wpcore.
type options struct{}

func (options) TaskTimeout(timeout time.Duration) wpcore.Option {
	return wpcore.WithTaskTimeout(timeout)
}

func (options) Recover(r wpcore.Recover) wpcore.Option {
	return wpcore.WithRecover(r)
}

func (options) ExitTogether() wpcore.Option {
	return wpcore.WithExitTogether()
}

func (options) WrapsChain(wraps ...wpcore.TaskWrap) wpcore.Option {
	return wpcore.WithWrapsChain(wraps...)
}

func (options) SkipPendingTask(asError bool) wpcore.Option {
	return wpcore.SkipPendingTask(asError)
}

func (options) ParallelLimit(limit uint) wpcore.Option {
	return wpcore.WithParallelLimit(limit)
}

// Deprecated: tasks will not be skipped by default now.
// Use SkipPendingTask(false) instead.
func (options) IgnoreSkippingPendingErr() wpcore.Option {
	return wpcore.SkipPendingTask(false)
}

// Deprecated: tasks will not be skipped by default now.
// If you want to skip the pending task when the context canceled, use SkipPendingTask() instead.
func (options) DontSkipTask() wpcore.Option {
	return wpcore.WithDontSkipTask()
}
