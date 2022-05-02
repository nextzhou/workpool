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

func WithIgnoreSkippingPendingErr() Option {
	return commonOption{
		f: func(w *Workpool) {
			w.conf.ignoreSkipping = true
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
