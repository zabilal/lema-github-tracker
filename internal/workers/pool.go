package workers

import (
	"sync"
)

// Task represents a unit of work
type Task func() error

// Pool manages a pool of worker goroutines
type Pool struct {
	tasks   chan Task
	wg      sync.WaitGroup
	errChan chan error
}

// NewPool creates a new worker pool
func NewPool(workerCount int) *Pool {
	p := &Pool{
		tasks:   make(chan Task),
		errChan: make(chan error, workerCount),
	}

	p.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go p.worker()
	}

	return p
}

// Submit adds a task to the pool
func (p *Pool) Submit(task Task) {
	p.tasks <- task
}

// Wait waits for all tasks to complete and returns any errors
func (p *Pool) Wait() []error {
	close(p.tasks)
	p.wg.Wait()
	close(p.errChan)

	var errors []error
	for err := range p.errChan {
		if err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		if err := task(); err != nil {
			p.errChan <- err
		}
	}
}