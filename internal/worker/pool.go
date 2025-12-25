package worker

import (
	"context"
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
    id         int
    taskQueue  chan Task
    quit       chan struct{}
    wg         *sync.WaitGroup
    isRunning  atomic.Bool
}

// WorkerPool manages multiple workers
type WorkerPool struct {
    workers    []*Worker
    taskQueue  chan Task
    wg         sync.WaitGroup
    maxWorkers int
    logger     *log.Logger
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(maxWorkers int) *WorkerPool {
    return &WorkerPool{
        workers:    make([]*Worker, 0, maxWorkers),
        taskQueue:  make(chan Task, 100), // Buffer tasks
        maxWorkers: maxWorkers,
        logger:     log.New(log.Writer(), "[WorkerPool] ", log.LstdFlags),
    }
}

// Start initializes and starts the worker pool
func (wp *WorkerPool) Start() {
    wp.logger.Printf("Starting worker pool with %d workers", wp.maxWorkers)
    
    for i := 0; i < wp.maxWorkers; i++ {
        worker := &Worker{
            id:        i + 1,
            taskQueue: wp.taskQueue,
            quit:      make(chan struct{}),
            wg:        &wp.wg,
        }
        wp.workers = append(wp.workers, worker)
        wp.wg.Add(1)
        go worker.start()
    }
}

// Submit adds a task to the queue
func (wp *WorkerPool) Submit(task Task) {
    wp.taskQueue <- task
}

// SubmitWithTimeout adds a task with timeout
func (wp *WorkerPool) SubmitWithTimeout(ctx context.Context, task Task, timeout time.Duration) error {
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
    wp.logger.Println("Shutting down worker pool...")
    
    // Stop accepting new tasks
    close(wp.taskQueue)
    
    // Signal all workers to stop
    for _, worker := range wp.workers {
        close(worker.quit)
    }
    
    // Wait for all workers to finish
    wp.wg.Wait()
    wp.logger.Println("Worker pool shutdown complete")
}

// Worker implementation
func (w *Worker) start() {
    defer w.wg.Done()
    w.isRunning.Store(true)
    
    log.Printf("Worker %d started", w.id)
    
    for {
        select {
        case task := <-w.taskQueue:
            w.executeTask(task)
        case <-w.quit:
            w.isRunning.Store(false)
            log.Printf("Worker %d stopped", w.id)
            return
        }
    }
}

func (w *Worker) executeTask(task Task) {
    start := time.Now()
    log.Printf("Worker %d processing task: %s", w.id, task.Name())
    
    var err error
    maxRetries := task.RetryCount()
    
    for attempt := 0; attempt <= maxRetries; attempt++ {
        if attempt > 0 {
            log.Printf("Worker %d retrying task %s (attempt %d/%d)", 
                w.id, task.Name(), attempt, maxRetries)
            time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
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