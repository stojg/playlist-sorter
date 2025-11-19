// ABOUTME: Simple worker pool for parallelizing batch tasks
// ABOUTME: Provides submit-and-wait pattern optimized for the genetic algorithm

package pool

import (
	"runtime"
	"sync"
)

// WorkerPool manages a pool of worker goroutines for parallel task execution
type WorkerPool struct {
	workers  int
	taskChan chan func()
	workerWg sync.WaitGroup // tracks worker goroutines lifetime
	taskWg   sync.WaitGroup // tracks submitted tasks completion
}

// NewWorkerPool creates a worker pool sized to available CPUs
// The bufferSize determines the task channel capacity
func NewWorkerPool(bufferSize int) *WorkerPool {
	numWorkers := runtime.NumCPU()
	pool := &WorkerPool{
		workers:  numWorkers,
		taskChan: make(chan func(), bufferSize),
	}

	// Start worker goroutines
	for range numWorkers {
		pool.workerWg.Add(1)

		go func() {
			defer pool.workerWg.Done()

			for task := range pool.taskChan {
				task()
				pool.taskWg.Done() // Mark task as complete
			}
		}()
	}

	return pool
}

// Submit adds a task to the pool
// Blocks if the task channel is full
func (p *WorkerPool) Submit(task func()) {
	p.taskWg.Add(1)
	p.taskChan <- task
}

// Wait blocks until all submitted tasks have completed
func (p *WorkerPool) Wait() {
	p.taskWg.Wait()
}

// Close shuts down the worker pool and waits for all workers to exit
func (p *WorkerPool) Close() {
	close(p.taskChan)
	p.workerWg.Wait()
}
