//nolint:gochecknoglobals,funlen
package workpool

import (
	"context"
	"errors"
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	emptyTask Task = func(ctx context.Context) error { return nil }
	blockTask Task = func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}
)

var panicTask Task = func(ctx context.Context) error {
	panic("foo")
}

var errTask Task = func(ctx context.Context) error {
	return errors.New("bar") //nolint:goerr113
}

var (
	sleepTime      = 100 * time.Millisecond
	sleepTask Task = func(ctx context.Context) error {
		select {
		case <-ctx.Done():
		case <-time.After(sleepTime):
		}
		return nil
	}
)

var ignoreCtxSleepTask Task = func(ctx context.Context) error {
	return sleepTask(context.Background())
}

type taskStat struct {
	totalNum        int64
	currentParallel int64
	maxParallel     int64
	mu              sync.Mutex
}

func (s *taskStat) wrap(task Task) Task {
	return func(ctx context.Context) error {
		parallel := atomic.AddInt64(&s.currentParallel, 1)
		s.mu.Lock()
		if parallel > s.maxParallel {
			s.maxParallel = parallel
		}
		s.mu.Unlock()
		defer func() {
			atomic.AddInt64(&s.totalNum, 1)
			atomic.AddInt64(&s.currentParallel, -1)
		}()
		return task(ctx)
	}
}

func TestWorkpool(t *testing.T) {
	Convey("workpool", t, func() {
		Convey("uninitialized workpool", func() {
			var w Workpool
			So(func() { _ = w.Wait() }, ShouldPanic)
		})
		Convey("reuse workpool", func() {
			w := New(context.Background())
			So(w.Wait(), ShouldBeNil)
			So(func() { _ = w.Wait() }, ShouldPanic)
		})
		Convey("commonly workpool", func() {
			w := New(context.Background())
			w.Go(emptyTask)
			So(w.Wait(), ShouldBeNil)
		})
		Convey("take first error", func() {
			w := New(context.Background())
			for i := 0; i < 10; i++ {
				w.Go(func(ctx context.Context) error {
					time.Sleep(sleepTime)
					return errTask(ctx)
				})
			}
			So(w.Wait(), ShouldBeError, "bar")
		})
		Convey("panic on master goroutine", func() {
			w := New(context.Background())
			w.Go(panicTask)
			So(func() { _ = w.Wait() }, ShouldPanic)
		})
		Convey("context cancel tasks", func() {
			ctx, cancel := context.WithCancel(context.Background())
			w := New(ctx)
			w.Go(blockTask)
			runtime.Gosched()
			cancel()
			So(w.Wait(), ShouldBeNil)
		})
		Convey("task error cancel other tasks", func() {
			w := New(context.Background())
			w.Go(blockTask)
			w.Go(errTask)
			So(w.Wait(), ShouldBeError, "bar")
		})
		Convey("collect error", func() {
			w := New(context.Background())
			w.Go(blockTask)
			w.Go(errTask)
			w.Go(emptyTask)
			So(w.Wait(), ShouldBeError, "bar")
		})
		Convey("wait all task to finish", func() {
			start := time.Now()
			w := New(context.Background())
			var ts taskStat
			w.Go(ts.wrap(ignoreCtxSleepTask))
			So(w.Wait(), ShouldBeNil)
			So(time.Since(start), ShouldBeGreaterThanOrEqualTo, sleepTime)
			So(ts.totalNum, ShouldEqual, 1)
		})
		Convey("wait all tasks to finish, even if the ctx has already done", func() {
			ctx, cancel := context.WithCancel(context.Background())
			w := New(ctx)
			var ts taskStat
			w.Go(ts.wrap(ignoreCtxSleepTask))
			w.Go(ts.wrap(ignoreCtxSleepTask))
			runtime.Gosched()
			cancel()
			So(w.Wait(), ShouldBeNil)
			So(ts.totalNum, ShouldEqual, 2)
		})
		Convey("skip pending task error", func() {
			ctx, cancel := context.WithCancel(context.Background())
			w := New(ctx)
			w.Go(emptyTask)
			runtime.Gosched()
			cancel()
			w.Go(emptyTask)
			So(w.Wait(), ShouldBeError, "skip 1 pending tasks")
		})
	})
}

func TestWithParallelLimit(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	randomParallelNum := uint(rand.Uint32()%10) + 1 //nolint:gosec
	Convey("WithParallelLimit"+strconv.Itoa(int(randomParallelNum)), t, func() {
		var ts taskStat
		w := New(context.Background(), WithParallelLimit(randomParallelNum))
		for i := 0; i < 20; i++ {
			w.Go(ts.wrap(ignoreCtxSleepTask))
		}
		So(w.Wait(), ShouldBeNil)
		So(ts.maxParallel, ShouldBeLessThanOrEqualTo, randomParallelNum)
		So(ts.currentParallel, ShouldEqual, 0)
		So(ts.totalNum, ShouldEqual, 20)
		// t.Logf("limit: %d, max: %d\n", randomParallelNum, ts.maxParallel)
	})
}

func TestWithTaskTimeout(t *testing.T) {
	Convey("WithTaskTimeout", t, func() {
		w := New(context.Background(), WithTaskTimeout(time.Millisecond))
		w.Go(blockTask)
		So(w.Wait(), ShouldBeNil)
	})
}

func TestWithRecover(t *testing.T) {
	Convey("WithRecover", t, func() {
		Convey("convert panic to error", func() {
			var panicErr ErrPanic
			w := New(context.Background(), WithRecover(func(err ErrPanic) error {
				panicErr = err
				return errors.New("don't panic") //nolint:goerr113
			}))
			w.Go(panicTask)
			So(w.Wait(), ShouldBeError, "don't panic")
			So(panicErr.Recover, ShouldEqual, "foo")
			So(panicErr.Error(), ShouldContainSubstring, "foo")
		})
		Convey("convert panic to ok", func() {
			var panicErr ErrPanic
			w := New(context.Background(), WithRecover(func(err ErrPanic) error {
				panicErr = err
				return nil
			}))
			w.Go(panicTask)
			So(w.Wait(), ShouldBeNil)
			So(panicErr.Recover, ShouldEqual, "foo")
		})
		Convey("panic in custom recover", func() {
			var panicErr ErrPanic
			w := New(context.Background(), WithRecover(func(err ErrPanic) error {
				panicErr = err
				panic("panic in custom recover again")
			}))
			Convey("task panic", func() {
				w.Go(panicTask)
				So(func() { _ = w.Wait() }, ShouldPanic)
				So(panicErr.Recover, ShouldEqual, "foo")
			})
			Convey("task hasn't panic", func() {
				w.Go(emptyTask)
				So(w.Wait(), ShouldBeNil)
				So(panicErr.Recover, ShouldBeNil)
			})
		})
	})
}

func TestWithExitTogether(t *testing.T) {
	Convey("WithExitTogether", t, func() {
		Convey("single task exit", func() {
			w := New(context.Background(), WithExitTogether())
			start := time.Now()
			w.Go(sleepTask)
			So(w.Wait(), ShouldBeNil)
			So(time.Since(start), ShouldBeGreaterThanOrEqualTo, sleepTime)
		})
		Convey("non error exit", func() {
			w := New(context.Background(), WithExitTogether(), WithIgnoreSkippingPendingErr())
			start := time.Now()
			w.Go(sleepTask)
			w.Go(emptyTask)
			So(w.Wait(), ShouldBeNil)
			So(time.Since(start), ShouldBeLessThan, sleepTime)
		})
		Convey("error exit", func() {
			w := New(context.Background(), WithExitTogether())
			start := time.Now()
			w.Go(sleepTask)
			w.Go(errTask)
			So(w.Wait(), ShouldBeError, "bar")
			So(time.Since(start), ShouldBeLessThan, sleepTime)
		})
		Convey("panic exit", func() {
			w := New(context.Background(), WithExitTogether())
			start := time.Now()
			w.Go(sleepTask)
			w.Go(panicTask)
			So(func() { _ = w.Wait() }, ShouldPanic)
			So(time.Since(start), ShouldBeLessThan, sleepTime)
		})
		Convey("panic as error exit", func() {
			w := New(context.Background(), WithExitTogether(), WithChain(PanicAsError), WithIgnoreSkippingPendingErr())
			start := time.Now()
			w.Go(sleepTask)
			w.Go(panicTask)
			err := w.Wait()
			So(err, ShouldBeError)
			So(err, ShouldHaveSameTypeAs, ErrPanic{})
			So(time.Since(start), ShouldBeLessThan, sleepTime)
		})
		Convey("different error types", func() {
			for i := 0; i < 10000; i++ {
				w := New(context.Background(), WithChain(PanicAsError))
				w.Go(panicTask)
				w.Go(errTask)
				_ = w.Wait()
			}
		})
		Convey("ignore panic exit", func() {
			w := New(context.Background(), WithExitTogether(), WithRecover(func(err ErrPanic) error {
				return nil
			}))
			start := time.Now()
			w.Go(sleepTask)
			w.Go(panicTask)
			So(w.Wait(), ShouldBeNil)
			So(time.Since(start), ShouldBeLessThan, sleepTime)
		})
	})
}

func TestWithIgnoreSkippingPendingErr(t *testing.T) {
	Convey("WithIgnoreSkippingPendingErr", t, func() {
		Convey("don't ignore", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := New(ctx)
			var ts taskStat
			rand.Seed(time.Now().UnixNano())
			cancelIdx := rand.Intn(100) //nolint:gosec
			for i := 0; i < 100; i++ {
				if i == cancelIdx {
					cancel()
				}
				w.Go(ts.wrap(emptyTask))
			}
			err := w.Wait()
			So(err, ShouldBeError)
			var skipErr ErrSkipPendingTask
			So(errors.As(err, &skipErr), ShouldBeTrue)
			So(skipErr.SKippingTaskCount, ShouldEqual, 100-ts.totalNum)
		})
		Convey("ignore", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := New(ctx, WithIgnoreSkippingPendingErr())
			var ts taskStat
			w.Go(ts.wrap(emptyTask))
			So(w.Wait(), ShouldBeNil)
			So(ts.totalNum, ShouldEqual, 0)
		})
	})
}

func TestWithChain(t *testing.T) {
	Convey("WithChain", t, func() {
		var processMarks []int
		w1 := TaskWrapper(func(task Task) Task {
			return func(ctx context.Context) error {
				processMarks = append(processMarks, 1)
				err := task(ctx)
				processMarks = append(processMarks, 1)
				return err
			}
		})
		w2 := TaskWrapper(func(task Task) Task {
			return func(ctx context.Context) error {
				processMarks = append(processMarks, 2)
				err := task(ctx)
				processMarks = append(processMarks, 2)
				return err
			}
		})
		w3 := TaskWrapper(func(task Task) Task {
			return func(ctx context.Context) error {
				processMarks = append(processMarks, 3)
				err := task(ctx)
				processMarks = append(processMarks, 3)
				return err
			}
		})
		w := New(context.Background(), WithChain(w1, w2, w3))
		w.Go(func(context.Context) error {
			processMarks = append(processMarks, 0)
			return nil
		})
		So(w.Wait(), ShouldBeNil)
		So(processMarks, ShouldResemble, []int{1, 2, 3, 0, 3, 2, 1})
	})
}

func TestTask_Go(t *testing.T) {
	Convey("go single task without pool", t, func() {
		Convey("just go", func() {
			wait := emptyTask.Go(context.Background())
			So(wait, ShouldNotBeNil)
			So(wait(), ShouldBeNil)
		})
		Convey("wait the task done", func() {
			start := time.Now()
			wait := sleepTask.Go(context.Background())
			So(wait(), ShouldBeNil)
			So(time.Since(start), ShouldBeGreaterThanOrEqualTo, sleepTime)
		})
		Convey("context cancel the task", func() {
			ctx, cancel := context.WithCancel(context.Background())
			wait := blockTask.Go(ctx)
			cancel()
			So(wait(), ShouldBeNil)
		})
		Convey("take task error", func() {
			wait := errTask.Go(context.Background())
			So(wait(), ShouldBeError)
		})
		Convey("panic on wait function", func() {
			wait := panicTask.Go(context.Background())
			So(func() { _ = wait() }, ShouldPanic)
		})
		Convey("panic as error", func() {
			task := PanicAsError(panicTask)
			wait := task.Go(context.Background())
			err := wait()
			So(err, ShouldBeError)
			So(err, ShouldHaveSameTypeAs, ErrPanic{})
		})
	})
}

func TestPhasedTask(t *testing.T) {
	milestone1, milestone2 := "foo", "bar"
	Convey("phased task", t, func() {
		Convey("milestone", func() {
			task, supervisor := Phased(func(ctx context.Context, helper PhasedTaskHelper) error {
				helper.MarkAMilestone(milestone1)
				return nil
			})
			wait := task.Go(context.Background())

			milestone, status := supervisor.WaitMilestone(context.Background())
			So(status.IsOK(), ShouldBeTrue)
			So(milestone, ShouldEqual, milestone1)
			So(wait(), ShouldBeNil)
		})

		Convey("wait a milestone", func() {
			task, supervisor := Phased(func(ctx context.Context, helper PhasedTaskHelper) error {
				helper.MarkAMilestone(milestone1)
				time.Sleep(sleepTime)
				helper.MarkAMilestone(milestone2)
				return nil
			})
			wait := task.Go(context.Background())
			milestone, status := supervisor.WaitMilestone(context.Background())
			startAt := time.Now()

			So(time.Since(startAt), ShouldBeLessThan, sleepTime)
			So(status.IsOK(), ShouldBeTrue)
			So(milestone, ShouldEqual, milestone1)

			ctx, _ := context.WithTimeout(context.Background(), 0) //nolint: govet
			milestone, status = supervisor.WaitMilestone(ctx)

			So(time.Since(startAt), ShouldBeLessThan, sleepTime)
			So(status.IsOK(), ShouldBeFalse)
			So(milestone, ShouldBeNil)
			So(status, ShouldEqual, phasedTaskStatusContextDone)

			ctx, _ = context.WithTimeout(context.Background(), 2*sleepTime) //nolint: govet
			milestone, status = supervisor.WaitMilestone(ctx)

			So(time.Since(startAt), ShouldBeBetween, sleepTime, 2*sleepTime)
			So(status.IsOK(), ShouldBeTrue)
			So(milestone, ShouldEqual, milestone2)

			ctx, _ = context.WithTimeout(context.Background(), sleepTime) //nolint: govet
			milestone, status = supervisor.WaitMilestone(ctx)

			So(status, ShouldEqual, phasedTaskStatusTaskDone)
			So(milestone, ShouldBeNil)

			So(wait(), ShouldBeNil)
		})

		Convey("phased task without milestone", func() {
			task, supervisor := Phased(func(ctx context.Context, helper PhasedTaskHelper) error {
				return nil
			})
			task.Go(context.Background())
			ctx, _ := context.WithTimeout(context.Background(), sleepTime) //nolint: govet
			milestone, status := supervisor.WaitMilestone(ctx)
			So(status, ShouldEqual, phasedTaskStatusTaskDone)
			So(milestone, ShouldBeNil)
		})
		Convey("cancel task when milestone timeout", func() {
			task, supervisor := Phased(func(ctx context.Context, helper PhasedTaskHelper) error {
				helper.MarkAMilestone(milestone1)
				<-ctx.Done()
				helper.MarkAMilestone(milestone2)
				return nil
			})

			wait := task.Go(context.Background())

			ctx, _ := context.WithTimeout(context.Background(), sleepTime) //nolint: govet
			milestone, status := supervisor.WaitMilestoneOrCancel(ctx)
			So(status.IsOK(), ShouldBeTrue)
			So(milestone, ShouldEqual, milestone1)

			milestone, status = supervisor.WaitMilestoneOrCancel(ctx)
			So(status, ShouldEqual, phasedTaskStatusContextDone)
			So(milestone, ShouldBeNil)

			So(wait(), ShouldBeNil)
		})
		Convey("skipped phased task", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			task, supervisor := Phased(func(ctx context.Context, helper PhasedTaskHelper) error {
				helper.MarkAMilestone(milestone1)
				return nil
			})

			wp := New(ctx)
			wp.Go(task)
			milestone, status := supervisor.WaitMilestone(ctx)
			So(milestone, ShouldBeNil)
			So(status.IsContextDone(), ShouldBeTrue)
			So(status.IsTaskNotRunning(), ShouldBeTrue)
			So(wp.Wait(), ShouldBeError, "skip 1 pending tasks")
		})
		Convey("not-running phased task", func() {
			_, supervisor := Phased(func(ctx context.Context, helper PhasedTaskHelper) error {
				helper.MarkAMilestone(milestone1)
				return nil
			})
			ctx, _ := context.WithTimeout(context.Background(), time.Millisecond) //nolint: govet
			milestone, status := supervisor.WaitMilestoneOrCancel(ctx)
			So(milestone, ShouldBeNil)
			So(status.IsContextDone(), ShouldBeTrue)
			So(status.IsTaskNotRunning(), ShouldBeTrue)
		})
	})
}

func BenchmarkGo0(b *testing.B) {
	b.ReportAllocs()
	wp := New(context.Background())
	for i := 0; i < b.N; i++ {
		wp.Go(func(ctx context.Context) error {
			return nil
		})
	}
	_ = wp.Wait()
}

func BenchmarkGo1(b *testing.B) {
	b.ReportAllocs()
	wp := New(context.Background(), WithParallelLimit(1))
	for i := 0; i < b.N; i++ {
		wp.Go(emptyTask)
	}
	_ = wp.Wait()
}

func BenchmarkGo10(b *testing.B) {
	b.ReportAllocs()
	wp := New(context.Background(), WithParallelLimit(10))
	for i := 0; i < b.N; i++ {
		wp.Go(emptyTask)
	}
	_ = wp.Wait()
}

func BenchmarkGo100(b *testing.B) {
	b.ReportAllocs()
	wp := New(context.Background(), WithParallelLimit(100))
	for i := 0; i < b.N; i++ {
		wp.Go(emptyTask)
	}
	_ = wp.Wait()
}

func BenchmarkGo1000(b *testing.B) {
	b.ReportAllocs()
	wp := New(context.Background(), WithParallelLimit(1000))
	for i := 0; i < b.N; i++ {
		wp.Go(emptyTask)
	}
	_ = wp.Wait()
}
