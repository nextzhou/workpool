package workpool

import (
	"context"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type Workpool struct {
	conf           conf
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	taskC          chan Task
	err            atomic.Value
	unhandledPanic atomic.Value
	skippingNum    uint64
}

type conf struct {
	taskChain      []TaskWrapper
	taskTimeout    time.Duration
	parallelLimit  int
	recover        Recover
	exitTogether   bool
	ignoreSkipping bool
}

func New(ctx context.Context, opts ...Option) *Workpool {
	w := &Workpool{
		taskC: make(chan Task),
	}
	w.ctx, w.cancel = context.WithCancel(ctx)
	for _, opt := range opts {
		opt.apply(w)
	}
	w.conf.recover = w.wrapRecover(w.conf.recover) // recover of custom recover

	w.wg.Add(w.conf.parallelLimit)
	for i := 0; i < w.conf.parallelLimit; i++ {
		go w.workLoop()
	}

	return w
}

func (w *Workpool) Go(task Task) {
	if w.ctx.Err() != nil {
		atomic.AddUint64(&w.skippingNum, 1)
		return
	}

	if w.conf.parallelLimit != 0 {
		w.taskC <- task
		return
	}

	for {
		select {
		case w.taskC <- task:
			return
		default:
			w.wg.Add(1)
			go w.workLoop()
			runtime.Gosched()
		}
	}
}

func (w *Workpool) Wait() error {
	close(w.taskC)
	w.wg.Wait()

	if err, ok := w.unhandledPanic.Load().(ErrPanic); ok {
		panic(err)
	}

	err, _ := w.err.Load().(uniformError)

	if err.error == nil && w.skippingNum > 0 && !w.conf.ignoreSkipping {
		return ErrSkipPendingTask{SKippingTaskCount: uint(w.skippingNum)}
	}
	return err.error
}

func (w *Workpool) workLoop() {
	defer func() {
		if w.conf.exitTogether {
			w.cancel()
		}
		r := recover()
		if r == nil {
			w.wg.Done()
			return
		}

		stack := debug.Stack()
		err := ErrPanic{Recover: r, Stack: stack}
		if err := w.conf.recover(err); err != nil {
			w.setErr(err)
		}
		go w.workLoop()
	}()

	for task := range w.taskC {
		err := w.runTask(task)
		if w.conf.exitTogether {
			w.cancel()
		}
		if err != nil {
			w.setErr(err)
			continue
		}
	}
}

func (w *Workpool) setErr(err error) {
	if !w.err.CompareAndSwap(nil, uniformError{err}) {
		return
	}
	w.cancel()
}

func (w *Workpool) runTask(task Task) error {
	ctx := w.ctx
	if ctx.Err() != nil {
		atomic.AddUint64(&w.skippingNum, 1)
		return nil
	}
	if timeout := w.conf.taskTimeout; timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	for _, wrapper := range w.conf.taskChain {
		task = wrapper(task)
	}

	return task(ctx)
}

func (w *Workpool) wrapRecover(r Recover) Recover {
	if r == nil {
		return func(err ErrPanic) error {
			w.unhandledPanic.CompareAndSwap(nil, err)
			return err
		}
	}
	return func(err ErrPanic) (newErr error) {
		defer func() {
			if r := recover(); r != nil {
				newErr = ErrPanic{Recover: r, Stack: debug.Stack()}
				w.unhandledPanic.CompareAndSwap(nil, newErr)
			}
		}()
		newErr = r(err)
		return
	}
}
