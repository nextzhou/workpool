# WorkPool
[![Language](https://img.shields.io/badge/Language-Go-blue.svg)](https://golang.org/)
[![Go](https://github.com/nextzhou/workpool/actions/workflows/go.yml/badge.svg)](https://github.com/nextzhou/workpool/actions/workflows/go.yml)
[![golangci-lint](https://github.com/nextzhou/workpool/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/nextzhou/workpool/actions/workflows/golangci-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nextzhou/workpool)](https://goreportcard.com/report/github.com/nextzhou/workpool)

[中文文档](README.cn.md)

Workpool is a structured concurrency library that implements a [fork-join](https://zh.wikipedia.org/wiki/Fork-join%E6%A8%A1%E5%9E%8B) model, making concurrent tasks safer and more controllable.

```go
// Create a new Workpool and limit the maximum concurrency to 4.
wp := workpool.New(context.TODO(), workpool.Options.ParallelLimit(4))

for _, task := range tasks {
    task := task // Shadowing the task variable
    wp.Go(func(ctx context.Context) error { // Execute the subtask asynchronously here.
        return task(ctx)
    })
}

err := wp.Wait() // Wait for all tasks to complete here and handle errors and panics.
```

## Core features:

- [x] Lightweight fork-join concurrency model with lazy expansion of worker goroutines.
- [x] Collects errors and panics from subtasks and aggregates them in the `Workpool.Wait()` function.
- [x] Controls the lifecycle of subtasks through `Context`, ensuring that all worker goroutines are immediately released when `Workpool.Wait()` is called.
- [x] A single `Task` detached from the `Workpool` can also be executed asynchronously safely.
- [x] Supports phased tasks and can interactively obtain phased results from asynchronous tasks.
- [ ] Supports producer-consumer tasks based on `channel`, with automatic notification to consumer tasks after all producer tasks are completed (requires generics).
 
## Design

`New()`, `Go()`, and `Wait()` correspond respectively to `config`, `fork`, and `join`.

### Options

`Option` can be passed into `New()` function, for example, `wp := New(ctx, Options.TaskTimeout(time.Second), Options.Chain(Wraps.PanicAsErr))`.

| Option                                 | Description                                                                                                                                                          |
|:---------------------------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Options.TaskTimeout(time.Duration)     | Set an independent timeout for each task.                                                                                                                            |
| Options.ParallelLimit(uint)            | Set the maximum concurrency for subtasks.                                                                                                                            |
| Options.ExitTogether()                 | Notify other subtasks to exit when any subtask completes, usually used when starting multiple resident services.                                                     |
| Options.WrapsChain(...wpcore.TaskWrap) | Add the passed `wpcore.TaskWrap` to each `Task`, with the effect applied from left to right.                                                                         |
| Options.Recover(wpcore.Recover)        | Customize how to handle a panic in a subtask.                                                                                                                        |
| Options.SkipPendingTask(bool)          | By default, even if `ctx` is finished, the subsequent `Task` will not be skipped. With this option, new `Task` added after the end of `ctx` can be skipped directly. |

### Wraps

`TaskWrap` wraps a `Task` into a new `Task`, such as recording metrics and so on. It can be extended as needed.

It is usually used in conjunction with `Options.WrapsChain()` to be automatically applied to all `Task`.

```go
wp := New(ctx, Options.WrapsChain(Wraps.PanicAsErr)) // Used in conjunction with `Options.WrapsChain()`.

wp.Go(Wraps.PanicAsErr(task))               // Used on a single `Task`.
```

| TaskWrapper        | Description                                                                                                                                                       |
|:-------------------|:------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Wraps.PanicAsError | Converts panic in child tasks into an error.                                                                                                                      |
| Wraps.Phased       | Converts phased tasks into normal tasks. See [Phased Task](#phased-task) for details.                                                                                        |
| Wraps.RunStopTask | Converts tasks that stop execution into tasks controlled by `ctx`. See [Task with Separate Stop Function](#task-with-separate-stop-function) for details. |

## Single Task

Sometimes, we just need to asynchronously execute a single task and check its result afterwards.
In this case, using a `Workpool` might be too cumbersome.

However, we can still use `Task.Go(context.Context)` to start an asynchronous task without creating a new `Workpool`.
This function returns a `wpcore.TaskWait`, which is an alias for `func() error`.
When we execute the returned `TaskWait`, it will wait for the task to complete and return the result.

```go
task := workpool.Task(func(context.Context) error {
    // Order a coffee.
})
waitCoffee := task.Go(context.TODO())

// Save the world.
// Blah blah blah.

if err := waitCoffee(); err == nil {
    // Enjoy your coffee.
}
```

As with executing `Task` in a `Workpool`,
all errors or panics in the `Task` will be collected and thrown in `Wait()` when executing asynchronously using `Task.Go(context.Context)`.
Additionally, you can also wrap the single task to be executed asynchronously using `Wraps.PanicAsError`.

## Phased Task

Phased tasks provide a way to interact with asynchronous tasks. The concept is best explained with an example:

> Suppose we have an asynchronous task that updates data on a timer, but the first update must succeed when the task starts.

Without a phased task, a common solution would be to perform the initial update synchronously and then start the remaining updates as an asynchronous `Task`.

```go
// Constructing `wp`, `ctx`...

err := initTask(ctx)
if err != nil {
    return err
}

wp.Go(func(ctx context.Context) error { 
    // Task blah blah
})
```

But the problem with this approach is that the initialization part cannot be processed asynchronously (which is necessary if there are multiple such tasks),
and the logic of a single task is fragmented, making it difficult to maintain.

With phased tasks, this problem can be easily solved:

```go
// Constructing `wp`, `ctx`...

task, supervisor := Wraps.Phased(func(ctx context.Context, helper wpcore.PhasedTaskHelper) error {
    err := taskInit(ctx)
    if err != nil {
    	return err
    }
    
    // Task initialization is complete. Let's mark this milestone.
    helper.MarkAMilestone(taskInitOk) 
    
    // Process the remaining parts of the task.
})

wp.Go(task)

initResult, status := supervisor.WaitMilestoneOrCancel(ctx)
```

In phased tasks, we can use `helper.MarkAMilestone(interface{})` to record phased achievements by calling it 0 or more times.
This is similar to the yield operation in generators in other languages,
but the difference is that phased tasks do not suspend after `MarkAMilestone`,
but continue to execute.

Outside of the task, we can interact with the task through the `PhasedTaskSupervisor` returned by `Wraps.Parsed()`,
achieving operations such as confirming phased achievements or setting deadlines for phased achievements to cancel if exceeded:

|Function| Description                |
|:---|:------------------|
|WaitMilestone| Wait for a milestone to be reached.           |
|WaitMilestoneOrCancel| Wait for a milestone to be reached, and cancel the task if the deadline is exceeded. |

In addition, the `WaitMilestone` series of functions also returns a `PhasedTaskStatus`,
which can be used to determine the state at the time the function returns:

|Status|Description|Note|
|:---|:---|:----|
|IsOK()|Successfully reached a milestone.||
|IsContextDone()|Context is done and the milestone was not reached.|May coexist with `IsTaskNotRunning()`.|
|IsTaskDone()|The task has finished, but no milestone was reached.||
|IsTaskNotRunning()|The task has not started running when the context is done.|Will always coexist with `IsContextDone()`.|

## Task with Separate Stop Function

Some existing long-running services are not controlled by ctx for stopping, 
but instead provide a separate `Stop`/`Close` function to control shutdown.

For example, `http.Server` starts the service through the `Serve()` function
and stops the service through `net.Listener.Close()`.

`Wraps.RunStopTask()` provides a simple wrapper to convert this type of task into a `workpool.Task`.

Taking `http.Server` as an example, the following code can be used to convert it to a `workpool.Task`:

```go
task := Wraps.RunStopTask(func() error { // Running function.
    err := srv.Serve(l)
    if errors.Is(err, http.ErrServerClosed) { // Ignore the ServerClosed error.
        return nil
    }
    return err
}, func() error { // Stopping function.
    return l.Close()
})
```