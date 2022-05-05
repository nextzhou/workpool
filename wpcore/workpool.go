package wpcore

import (
	"context"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type Workpool struct {
	conf           conf
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	taskC          chan Task
	err            unsafe.Pointer // *UniformError
	unhandledPanic unsafe.Pointer // *ErrPanic
	skippingNum    uint64
}

type conf struct {
	taskChain      []TaskWrap
	taskTimeout    time.Duration
	parallelLimit  int
	recover        Recover
	exitTogether   bool
	ignoreSkipping bool
	dontSkip       bool
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
	if w.ctx.Err() != nil && !w.conf.dontSkip {
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

	if ptr := atomic.LoadPointer(&w.unhandledPanic); ptr != nil {
		panic(*(*ErrPanic)(ptr))
	}

	var err UniformError
	if ptr := atomic.LoadPointer(&w.err); ptr != nil {
		err = *(*UniformError)(ptr)
	}

	if err.Err == nil && w.skippingNum > 0 && !w.conf.ignoreSkipping {
		return ErrSkipPendingTask{SKippingTaskCount: uint(w.skippingNum)}
	}
	return err.Err
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
	if !atomic.CompareAndSwapPointer(&w.err, nil, unsafe.Pointer(&UniformError{Err: err})) {
		return
	}
	w.cancel()
}

func (w *Workpool) runTask(task Task) error {
	ctx := w.ctx
	if ctx.Err() != nil && !w.conf.dontSkip {
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
			heapErr := new(ErrPanic)
			*heapErr = err
			atomic.CompareAndSwapPointer(&w.unhandledPanic, nil, unsafe.Pointer(heapErr))
			return err
		}
	}
	return func(err ErrPanic) (newErr error) {
		defer func() {
			if r := recover(); r != nil {
				newErr := new(ErrPanic)
				*newErr = ErrPanic{Recover: r, Stack: debug.Stack()}
				atomic.CompareAndSwapPointer(&w.unhandledPanic, nil, unsafe.Pointer(newErr))
			}
		}()
		newErr = r(err)
		return
	}
}
