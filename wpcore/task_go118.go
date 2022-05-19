//go:build go1.18
// +build go1.18

package wpcore

import (
	"context"
	"sync"
)

type ProduceTask[T any] func(context.Context) (T, error)
type ConsumeTask[T any] func(collect <-chan T) error

func AsProduceConsumePool[T any](wp *Workpool) *ProduceConsumePool[T] {
	return &ProduceConsumePool[T]{
		wp:      wp,
		collect: make(chan T, 1),
	}
}

type ProduceConsumePool[T any] struct {
	producerWg sync.WaitGroup
	once       sync.Once
	wp         *Workpool
	collect    chan T
}

func (p *ProduceConsumePool[T]) GoProduce(task ProduceTask[T]) {
	p.producerWg.Add(1)
	p.wp.Go(func(ctx context.Context) error {
		defer p.producerWg.Done()

		item, err := task(ctx)
		if err != nil {
			return err
		}

		select {
		case p.collect <- item:
		case <-ctx.Done(): // TODO: is necessary?
		}
		return nil
	})
}

func (p *ProduceConsumePool[T]) GoConsume(task ConsumeTask[T]) {
	p.once.Do(func() {
		p.wp.Go(func(ctx context.Context) error {
			p.producerWg.Wait()
			close(p.collect)
			return nil
		})
	})

	p.wp.Go(func(context.Context) error {
		return task(p.collect)
	})
}
