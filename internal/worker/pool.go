package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	applog "wisdomHouse-backend/internal/logger"
)

// Task represents a job to be executed
type Task interface {
	Execute() error
	Name() string
	RetryCount() int
}

// Worker represents a single worker
type Worker struct {
	id        int
	taskQueue chan Task
	wg        *sync.WaitGroup
	isRunning atomic.Bool
}

// WorkerPool manages multiple workers
type WorkerPool struct {
	workers    []*Worker
	taskQueue  chan Task
	wg         sync.WaitGroup
	maxWorkers int
	started    atomic.Bool
	stopped    atomic.Bool
	stopOnce   sync.Once
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(maxWorkers int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 2
	}
	return &WorkerPool{
		workers:    make([]*Worker, 0, maxWorkers),
		taskQueue:  make(chan Task, 100),
		maxWorkers: maxWorkers,
	}
}

// Start initializes and starts the worker pool
func (wp *WorkerPool) Start() {
	if wp.started.Load() {
		return
	}
	wp.started.Store(true)
	applog.L().Info("starting worker pool", "workers", wp.maxWorkers)

	for i := 0; i < wp.maxWorkers; i++ {
		worker := &Worker{
			id:        i + 1,
			taskQueue: wp.taskQueue,
			wg:        &wp.wg,
		}
		wp.workers = append(wp.workers, worker)
		wp.wg.Add(1)
		go worker.start()
	}
}

// Submit adds a task to the queue
func (wp *WorkerPool) Submit(task Task) error {
	if task == nil {
		return errors.New("task cannot be nil")
	}
	if !wp.started.Load() {
		return errors.New("worker pool is not started")
	}
	if wp.stopped.Load() {
		return errors.New("worker pool is stopped")
	}

	select {
	case wp.taskQueue <- task:
		return nil
	default:
		return errors.New("worker queue is full")
	}
}

// SubmitWithTimeout adds a task with timeout
func (wp *WorkerPool) SubmitWithTimeout(ctx context.Context, task Task, timeout time.Duration) error {
	if task == nil {
		return errors.New("task cannot be nil")
	}
	if !wp.started.Load() {
		return errors.New("worker pool is not started")
	}
	if wp.stopped.Load() {
		return errors.New("worker pool is stopped")
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	select {
	case wp.taskQueue <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(timeout):
		return fmt.Errorf("task submission timeout")
	}
}

// Shutdown gracefully stops the worker pool
func (wp *WorkerPool) Shutdown() {
	wp.stopOnce.Do(func() {
		applog.L().Info("shutting down worker pool")
		wp.stopped.Store(true)
		close(wp.taskQueue)
		wp.wg.Wait()
		applog.L().Info("worker pool shutdown complete")
	})
}

// Worker implementation
func (w *Worker) start() {
	defer w.wg.Done()
	w.isRunning.Store(true)

	applog.L().Info("worker started", "worker_id", w.id)
	defer func() {
		w.isRunning.Store(false)
		applog.L().Info("worker stopped", "worker_id", w.id)
	}()

	for task := range w.taskQueue {
		if task == nil {
			continue
		}
		w.executeTask(task)
	}
}

func (w *Worker) executeTask(task Task) {
	start := time.Now()
	applog.L().Info("worker processing task", "worker_id", w.id, "task", task.Name())

	var err error
	maxRetries := task.RetryCount()
	if maxRetries < 0 {
		maxRetries = 0
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			applog.L().Info("worker retrying task", "worker_id", w.id, "task", task.Name(), "attempt", attempt, "max_retries", maxRetries)
			time.Sleep(time.Duration(1<<min(attempt, 5)) * 100 * time.Millisecond)
		}

		err = task.Execute()
		if err == nil {
			applog.L().Info("worker completed task", "worker_id", w.id, "task", task.Name(), "duration", time.Since(start))
			return
		}
	}

	applog.L().Warn("worker task failed", "worker_id", w.id, "task", task.Name(), "error", err)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
