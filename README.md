# WorkPool
[![Language](https://img.shields.io/badge/Language-Go-blue.svg)](https://golang.org/)
[![Go](https://github.com/nextzhou/workpool/actions/workflows/go.yml/badge.svg)](https://github.com/nextzhou/workpool/actions/workflows/go.yml)
[![golangci-lint](https://github.com/nextzhou/workpool/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/nextzhou/workpool/actions/workflows/golangci-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nextzhou/workpool)](https://goreportcard.com/report/github.com/nextzhou/workpool)

workpool 实现了一个 [fork-join](https://zh.wikipedia.org/wiki/Fork-join%E6%A8%A1%E5%9E%8B) 模型的并发控制库，使得并发任务更安全、可控。


```go
// 新建 Workpool，并限制最大并发数为 4
wp := workpool.New(context.TODO(), workpool.WithParallelLimit(4))

for _, task := range tasks {
    task := task // shadow task varible

    wp.Go(func(ctx context.Context) error { // 在这里异步执行子任务
        return task(ctx)
    })
}

err := wp.Wait() // 在这里等待所有任务完成，并处理错误与 panic
```

## 核心特性

- [x] 轻量级的 fork-join 并发模型，惰性扩展工作协程。
- [x] 收集子任务的错误与 panic，并在 `Workpool.Wait()` 函数中汇总。
- [x] 通过`Context`控制子任务生命周期，使得所有工作协程能保证在 `Workpool.Wait()` 都被即时释放。
- [ ] 支持基于 `channel` 生产-消费者 的任务，生产者任务全部完成后自动通知消费者任务（依赖泛型）

## 设计

 `New()`、`Go()`、`Wait()` 三段式分别对应 `config`、`fork`、`join`

### Option

`Option` 可在 `New()` 传入，例如 `wp := New(ctx, WithTaskTimeout(time.Second), WithChain(PanicAsErr))`

|Option|功能|
|:-----|:-----|
|WithTaskTimeout(time.Duration)|为每个任务设置独立的超时|
|WithParallelLimit(uint)|子任务最大并发限制|
|WithExitTogether()|当有任意子任务完成时通知其他子任务退出，一般在启动多个常驻服务时使用|
|WithChain(...TaskWrapper)|为每个`Task`添加传入的`TaskWrapper`，作用顺序从做至右|
|WithRecover(Recover)|自定义当子任务panic时如何处理|
|WithIgnoreSkippingPendingErr()|跳过了部分未执行任务不视为错误|

### TaskWrapper

`TaskWrapper` 将 `Task` 包装成新的 `Task`，例如记录 metrics 等等， 可以按照需求自行扩展。

一般与 `WithChain()` 配合使用，可自动应用到所有 `Task` 上。

```go
wp := New(ctx, WithChain(PanicAsErr)) // 配合 WithChain() 使用

wp.Go(PanicAsErr(task))               // 单独对某个 Task 使用
```

|TaskWrapper|功能|
|:----------|:---|
|PanicAsError|子任务 panic 会转换成错误|
