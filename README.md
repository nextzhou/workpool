# WorkPool
[![Language](https://img.shields.io/badge/Language-Go-blue.svg)](https://golang.org/)
[![Go](https://github.com/nextzhou/workpool/actions/workflows/go.yml/badge.svg)](https://github.com/nextzhou/workpool/actions/workflows/go.yml)
[![golangci-lint](https://github.com/nextzhou/workpool/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/nextzhou/workpool/actions/workflows/golangci-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nextzhou/workpool)](https://goreportcard.com/report/github.com/nextzhou/workpool)

workpool 实现了一个 [fork-join](https://zh.wikipedia.org/wiki/Fork-join%E6%A8%A1%E5%9E%8B) 模型的并发控制库，使得并发任务更安全、可控。


```go
// 新建 Workpool，并限制最大并发数为 4
wp := workpool.New(context.TODO(), workpool.Options.ParallelLimit(4))

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
- [x] 脱离 `Workpool` 的单个 `Task` 也可以安全地异步执行。
- [x] 分阶段任务，可交互式地从异步任务中获取阶段性结果
- [ ] 支持基于 `channel` 生产-消费者 的任务，生产者任务全部完成后自动通知消费者任务（依赖泛型）

## 设计

 `New()`、`Go()`、`Wait()` 三段式分别对应 `config`、`fork`、`join`

### Options

`Option` 可在 `New()` 传入，例如 `wp := New(ctx, Options.TaskTimeout(time.Second), Options.Chain(Wraps.PanicAsErr))`

| Option                                 | 功能                                       |
|:---------------------------------------|:-----------------------------------------|
| Options.TaskTimeout(time.Duration)     | 为每个任务设置独立的超时                             |
| Options.ParallelLimit(uint)            | 子任务最大并发限制                                |
| Options.ExitTogether()                 | 当有任意子任务完成时通知其他子任务退出，一般在启动多个常驻服务时使用       |
| Options.WrapsChain(...wpcore.TaskWrap) | 为每个`Task`添加传入的`wpcore.TaskWrap`，作用顺序从做至右 |
| Options.Recover(wpcore.Recover)        | 自定义当子任务panic时如何处理                        |
| Options.IgnoreSkippingPendingErr()     | 跳过了部分未执行任务不视为错误                          |

### Wraps

`TaskWrap` 将 `Task` 包装成新的 `Task`，例如记录 metrics 等等， 可以按照需求自行扩展。

一般与 `Options.WrapsChain()` 配合使用，可自动应用到所有 `Task` 上。

```go
wp := New(ctx, Options.WrapsChain(Wraps.PanicAsErr)) // 配合 Options.WrapsChain() 使用

wp.Go(Wraps.PanicAsErr(task))               // 单独对某个 Task 使用
```

| TaskWrapper        | 功能               |
|:-------------------|:-----------------|
| Wraps.PanicAsError | 子任务 panic 会转换成错误 |
| Wraps.Phased       | 将分阶段任务转成普通任务     |

## 单任务

有时只需要异步地执行单个任务，过后再检查其执行结果。 这时如果再使用 `Workpool` 就显得过于繁琐了。 

不过我们还可以调用 `Task.Go(context.Context)` 启动异步任务，而无需新建 `Workpool`。
该函数会返回一个`wpcore.TaskWait`，它是`func() error` 的别名，执行返回的 `TaskWait` 时会等待任务结束并返回结果。

```go
task := workpool.Task(func(context.Context) error {
    // order a coffee
})
waitCoffee := task.Go(context.TODO())

// save the world
// balabala

if err := waitCoffee(); err == nil {
    // enjoy your coffee
}
```

与在 `Workpool` 中执行 `Task` 一致，`Task` 中的所有错误或 `panic` 都会收集到 `wait()` 中抛出。同时你也可以使用 `Wraps.PanicAsError` 包装需要异步执行的单任务。

## 分阶段任务

分阶段任务提供一种与异步任务交互的手段，通过一个例子我们就很容易理解：

> 我们有一个异步执行的定时更新数据任务，但在启动时第一次更新必须成功。

在没有分阶段任务时常规的解决方法时将第一次更新单独执行，剩下的部分作为一个`Task`异步执行。
```go
// construct wp、ctx ...

err := initTask(ctx)
if err != nil {
    return err
}

wp.Go(func(ctx context.Context) error { 
    // task balabala
})
```
但这样的问题是初始化部分就无法也异步处理了(如果有多个这样的任务时是很有必要的)，
而且单个任务的逻辑被拆散，不方便维护。

如果有了分阶段任务，这个问题就很好解决了：

```go
// construct wp、ctx ...

task, supervisor := Wraps.Phased(func(ctx context.Context, helper wpcore.PhasedTaskHelper) error {
    err := taskInit(ctx)
    if err != nil {
    	return err
    }
    
    // task initialization done, mark a milestone
    helper.MarkAMilestone(taskInitOk) 
    
    // task balabala
})

wp.Go(task)

initResult, status := supervisor.WaitMilestoneOrCancel(ctx)
```

在分阶段任务中，我们可以通过调用 0 或多次 `helper.MarkAMilestone(interface{})`
来记录阶段性成果。
这有点类似于其他语言中 Generator 中的 yield 操作，
但区别在于分阶段任务在 `MarkAMileston` 之后并不会挂起，而是会继续执行。

在任务外，我们可以通过 `Wraps.Parsed()` 返回的 `PhasedTaskSupervisor` 来与任务交互，
达到确认阶段性成果、或者设置阶段性成果的 Deadline 超过则取消等操作：


|函数| 功能                |
|:---|:------------------|
|WaitMilestone| 等待一个里程碑           |
|WaitMilestoneOrCancel| 等待一个里程碑，若超时了则取消任务 |

另外，通过 `WaitMilestone` 系列函数中，除了返回里程碑还会返回一个 `PhasedTaskStatus`，
通过该值可以判断函数返回时的状态：

|状态|说明| 备注                        |
|:---|:---|:--------------------------|
|IsOK()| 成功取到里程碑|                           |
|IsContextDone()|ctx done 并且未能取到里程碑| 可能与 IsTaskNotRunning() 共存 |
|IsTaskDone()|任务结束了，但并没有产生里程碑||
|IsTaskNotRunning()| ctx done 时还任务还为开始运行| 一定会与 IsContextDone() 共存   |