package exec

import (
	"sync"
)

type Task interface {
	Apply() error
	HandleError(error)
	Prompt() string
}

type TaskQueue struct {
	queue   []Task
	mutex   sync.Mutex
	running bool
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		queue:   make([]Task, 0),
		running: false,
	}
}

func (q *TaskQueue) Enqueue(task Task) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.queue = append(q.queue, task)
	if !q.running {
		q.running = true
		go q.runLoop()
	}
}

func (q *TaskQueue) runLoop() {
	for {
		q.mutex.Lock()
		if len(q.queue) == 0 {
			q.running = false
			q.mutex.Unlock()
			return
		}
		task := q.queue[0]
		q.queue = q.queue[1:]
		q.mutex.Unlock()

		if err := task.Apply(); err != nil {
			task.HandleError(err)
		}
	}
}
