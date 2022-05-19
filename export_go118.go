//go:build go1.18
// +build go1.18

package workpool

import "github.com/nextzhou/workpool/wpcore"

func AsProduceConsumePool[T any](wp *wpcore.Workpool) *wpcore.ProduceConsumePool[T] {
	return wpcore.AsProduceConsumePool[T](wp)
}
