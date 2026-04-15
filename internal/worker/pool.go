package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
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
	logger     *log.Logger
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
		logger:     log.New(log.Writer(), "[WorkerPool] ", log.LstdFlags),
	}
}

// Start initializes and starts the worker pool
func (wp *WorkerPool) Start() {
	if wp.started.Load() {
		return
	}
	wp.started.Store(true)
	wp.logger.Printf("Starting worker pool with %d workers", wp.maxWorkers)

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
		wp.logger.Println("Shutting down worker pool...")
		wp.stopped.Store(true)
		close(wp.taskQueue)
		wp.wg.Wait()
		wp.logger.Println("Worker pool shutdown complete")
	})
}

// Worker implementation
func (w *Worker) start() {
	defer w.wg.Done()
	w.isRunning.Store(true)

	log.Printf("Worker %d started", w.id)
	defer func() {
		w.isRunning.Store(false)
		log.Printf("Worker %d stopped", w.id)
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
	log.Printf("Worker %d processing task: %s", w.id, task.Name())

	var err error
	maxRetries := task.RetryCount()
	if maxRetries < 0 {
		maxRetries = 0
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Worker %d retrying task %s (attempt %d/%d)",
				w.id, task.Name(), attempt, maxRetries)
			time.Sleep(time.Duration(1<<min(attempt, 5)) * 100 * time.Millisecond)
		}

		err = task.Execute()
		if err == nil {
			log.Printf("Worker %d completed task: %s in %v",
				w.id, task.Name(), time.Since(start))
			return
		}
	}

	log.Printf("Worker %d task failed: %s, error: %v", w.id, task.Name(), err)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
