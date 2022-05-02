package wpcore

import (
	"container/list"
	"context"
	"sync"
)

type PhasedTask func(ctx context.Context, helper PhasedTaskHelper) error

type PhasedTaskHelper struct {
	mu              *sync.Mutex
	milestones      *list.List
	milestoneNotify chan struct{}
	canceled        bool
	taskRunning     bool
}

func (h *PhasedTaskHelper) getMilestoneNotify() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.milestoneNotify
}

func (h *PhasedTaskHelper) MarkAMilestone(milestone interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.milestones.PushBack(milestone)
	close(h.milestoneNotify)
	h.milestoneNotify = make(chan struct{})
}

type PhasedTaskSupervisor struct {
	handler  *PhasedTaskHelper
	cancel   *context.CancelFunc
	taskDone chan struct{}
}

type PhasedTaskStatus uint8

const (
	phasedTaskStatusOK PhasedTaskStatus = iota
	phasedTaskStatusContextDone
	phasedTaskStatusTaskDone
	phasedTaskStatusContextDoneAndTaskNotRunning
)

func (s PhasedTaskStatus) IsOK() bool {
	return s == phasedTaskStatusOK
}

func (s PhasedTaskStatus) IsContextDone() bool {
	return s == phasedTaskStatusContextDone || s == phasedTaskStatusContextDoneAndTaskNotRunning
}

func (s PhasedTaskStatus) IsTaskDone() bool {
	return s == phasedTaskStatusTaskDone
}

func (s PhasedTaskStatus) IsTaskNotRunning() bool {
	return s == phasedTaskStatusContextDoneAndTaskNotRunning
}

func (o *PhasedTaskSupervisor) WaitMilestone(ctx context.Context) (interface{}, PhasedTaskStatus) {
	return o.waitMs(ctx, false)
}

func (o *PhasedTaskSupervisor) WaitMilestoneOrCancel(ctx context.Context) (interface{}, PhasedTaskStatus) {
	return o.waitMs(ctx, true)
}

func (o *PhasedTaskSupervisor) waitMs(ctx context.Context, cancelOnCtxDone bool) (interface{}, PhasedTaskStatus) {
	for {
		if elem := o.handler.popFrontMilestone(); elem != nil {
			return elem.Value, phasedTaskStatusOK
		}

		select {
		case <-ctx.Done():
			if elem := o.handler.popFrontMilestone(); elem != nil {
				return elem.Value, phasedTaskStatusOK
			}
			return o.onCtxDone(cancelOnCtxDone)
		case <-o.taskDone:
			if elem := o.handler.popFrontMilestone(); elem != nil {
				return elem.Value, phasedTaskStatusOK
			}
			return nil, phasedTaskStatusTaskDone
		case <-o.handler.getMilestoneNotify():
			continue
		}
	}
}

func (o *PhasedTaskSupervisor) onCtxDone(cancelOnCtxDone bool) (interface{}, PhasedTaskStatus) {
	o.handler.mu.Lock()
	running := o.handler.taskRunning
	if cancelOnCtxDone {
		o.handler.canceled = true
		if *o.cancel != nil {
			(*o.cancel)()
		}
	}
	o.handler.mu.Unlock()
	if running {
		return nil, phasedTaskStatusContextDone
	}
	return nil, phasedTaskStatusContextDoneAndTaskNotRunning
}

func (h *PhasedTaskHelper) popFrontMilestone() *list.Element {
	h.mu.Lock()
	defer h.mu.Unlock()

	front := h.milestones.Front()
	if front != nil {
		h.milestones.Remove(front)
	}

	return front
}

func Phased(task PhasedTask) (Task, PhasedTaskSupervisor) {
	handler := PhasedTaskHelper{
		mu:              new(sync.Mutex),
		milestones:      list.New(),
		milestoneNotify: make(chan struct{}),
	}
	supervisor := PhasedTaskSupervisor{
		handler:  &handler,
		taskDone: make(chan struct{}),
		cancel:   new(context.CancelFunc),
	}

	commonTask := func(ctx context.Context) error {
		defer func() { close(supervisor.taskDone) }()

		handler.mu.Lock()
		if handler.canceled {
			handler.mu.Unlock()
			return ErrSkipPendingTask{SKippingTaskCount: 1}
		}
		newCtx, cancel := context.WithCancel(ctx)
		*supervisor.cancel = cancel
		handler.taskRunning = true
		handler.mu.Unlock()

		return task(newCtx, handler)
	}

	return commonTask, supervisor
}
