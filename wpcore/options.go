package wpcore

import (
	"time"
)

type Option interface {
	apply(*Workpool)
}

type commonOption struct {
	f func(*Workpool)
}

func (o commonOption) apply(w *Workpool) {
	o.f(w)
}

func WithTaskTimeout(timeout time.Duration) Option {
	return commonOption{
		f: func(w *Workpool) {
			w.conf.taskTimeout = timeout
		},
	}
}

func WithRecover(r Recover) Option {
	return commonOption{
		f: func(w *Workpool) {
			w.conf.recover = r
		},
	}
}

func WithExitTogether() Option {
	return commonOption{
		f: func(w *Workpool) {
			w.conf.exitTogether = true
		},
	}
}

func WithWrapsChain(wrappers ...TaskWrap) Option {
	reverse := make([]TaskWrap, len(wrappers))
	for i := range wrappers {
		reverse[i] = wrappers[len(wrappers)-i-1]
	}

	return commonOption{
		f: func(w *Workpool) {
			w.conf.taskChain = reverse
		},
	}
}

func SkipPendingTask(asError bool) Option {
	return commonOption{
		f: func(w *Workpool) {
			w.conf.skipPending = true
			w.conf.skipAsErr = asError
		},
	}
}

func WithParallelLimit(limit uint) Option {
	return commonOption{
		f: func(w *Workpool) {
			w.conf.parallelLimit = int(limit)
		},
	}
}

// Deprecated: tasks will not be skipped by default now.
// Use SkipPendingTask(false) instead.
func WithIgnoreSkippingPendingErr() Option {
	return SkipPendingTask(false)
}

// Deprecated: tasks will not be skipped by default now.
func WithDontSkipTask() Option {
	return commonOption{
		f: func(w *Workpool) {
			w.conf.skipPending = false
		},
	}
}
