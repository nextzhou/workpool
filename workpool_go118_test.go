//go:build go1.18
// +build go1.18

package workpool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/nextzhou/workpool/wpcore"
	. "github.com/smartystreets/goconvey/convey"
)

func product(i int, hook func(i int) error) wpcore.ProduceTask[int] {
	return func(ctx context.Context) (int, error) {
		if hook != nil {
			err := hook(i)
			return i, err
		}
		return i, nil
	}
}

func consume(result *int64, hook func(i int) error) wpcore.ConsumeTask[int] {
	return func(c <-chan int) error {
		for n := range c {
			if hook != nil {
				if err := hook(n); err != nil {
					return err
				}
			}
			atomic.AddInt64(result, int64(n))
		}
		return nil
	}
}

func TestProduceConsume(t *testing.T) {
	Convey("AsProduceConsumePool", t, func() {
		Convey("just wait", func() {
			wp := New(context.Background())
			_ = AsProduceConsumePool[int](wp)
			So(wp.Wait(), ShouldBeNil)
		})
		Convey("single producer & single consumer", func() {
			wp := New(context.Background())
			pcp := AsProduceConsumePool[int](wp)
			pcp.GoProduce(product(123, nil))

			var result int64
			pcp.GoConsume(consume(&result, nil))
			So(wp.Wait(), ShouldBeNil)
			So(result, ShouldEqual, 123)
		})
		Convey("m producers & n consumers", func() {
			wp := New(context.Background())
			pcp := AsProduceConsumePool[int](wp)
			for i := 0; i < 10; i++ {
				pcp.GoProduce(product(i, nil))
			}
			var result int64
			for i := 0; i < 3; i++ {
				pcp.GoConsume(consume(&result, nil))
			}
			So(wp.Wait(), ShouldBeNil)
			So(result, ShouldEqual, 45)
		})
		Convey("produce error", func() {
			wp := New(context.Background())
			pcp := AsProduceConsumePool[int](wp)
			for i := 0; i < 10; i++ {
				pcp.GoProduce(product(i, func(i int) error {
					if i == 5 {
						return errors.New("foobar")
					}
					return nil
				}))
			}
			var result int64
			for i := 0; i < 3; i++ {
				pcp.GoConsume(consume(&result, nil))
			}
			So(wp.Wait(), ShouldBeError, "foobar")
		})
		Convey("all consume error", func() {
			wp := New(context.Background())
			pcp := AsProduceConsumePool[int](wp)
			for i := 0; i < 10; i++ {
				pcp.GoProduce(product(i, nil))
			}
			var result int64
			for i := 0; i < 3; i++ {
				pcp.GoConsume(consume(&result, func(i int) error {
					return errors.New("foobar")
				}))
			}
			So(wp.Wait(), ShouldBeError, "foobar")
		})
		SkipConvey("consumer panic but recovered", func() { //FIXME: exit producer when all consumer down
			wp := New(context.Background(), Options.Recover(func(err wpcore.ErrPanic) error {
				return nil // ignore panic
			}))
			pcp := AsProduceConsumePool[int](wp)
			for i := 0; i < 10; i++ {
				pcp.GoProduce(product(i, nil))
			}
			var result int64
			for i := 0; i < 3; i++ {
				pcp.GoConsume(consume(&result, func(i int) error {
					if i > 1 {
						panic("foobar")
					}
					return nil
				}))
			}
			So(wp.Wait(), ShouldBeNil)
			So(result, ShouldEqual, 1)
		})
		SkipConvey("ctx done before consume", func() { //FIXME: should consumer be skipped?
			ctx, cancel := context.WithCancel(context.Background())

			wp := New(ctx)
			pcp := AsProduceConsumePool[int](wp)
			waitProduce := make(chan struct{})
			pcp.GoProduce(product(123, func(i int) error {
				close(waitProduce)
				return nil
			}))
			<-waitProduce // make sure the producer was launched
			cancel()
			var result int64
			pcp.GoConsume(consume(&result, nil))
			So(wp.Wait(), ShouldBeError)
			So(result, ShouldEqual, 0)
		})
	})
}
