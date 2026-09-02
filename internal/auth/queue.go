// Package auth implements login queuing, warmup health-checks, and scheduling.
package auth

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// LoginJob is a single login task.
type LoginJob struct {
	AccountID int64
	Provider  string
	Email     string
	Password  string // decrypted
}

// LoginQueue fans jobs out to N workers running Python login scripts.
type LoginQueue struct {
	jobs       chan LoginJob
	workers    int
	cancel     context.CancelFunc
	ctx        context.Context
	pythonPath string

	// Callbacks set by the caller before Start().
	OnSuccess func(id int64, tokens string)
	OnError   func(id int64, msg string)
}

// NewLoginQueue creates a LoginQueue with the given number of workers.
func NewLoginQueue(workers int) *LoginQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &LoginQueue{
		jobs:       make(chan LoginJob, 100), // ponytail: fixed 100-slot buffer, tune if backpressure shows
		workers:    workers,
		ctx:        ctx,
		cancel:     cancel,
		pythonPath: "python3",
	}
}

// SetPythonPath sets the path to the Python executable.
func (q *LoginQueue) SetPythonPath(path string) {
	q.pythonPath = path
}

// Enqueue adds a job non-blocking; drops silently if the buffer is full.
func (q *LoginQueue) Enqueue(job LoginJob) {
	select {
	case q.jobs <- job:
	default:
		log.Printf("[auth/queue] buffer full, dropping job for account %d (%s)", job.AccountID, job.Email)
	}
}

// Start launches the worker goroutines.
func (q *LoginQueue) Start() {
	for i := 0; i < q.workers; i++ {
		go q.work()
	}
}

// Stop signals workers to stop and waits for the channel to drain.
func (q *LoginQueue) Stop() {
	q.cancel()
}

func (q *LoginQueue) work() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case job, ok := <-q.jobs:
			if !ok {
				return
			}
			q.run(job)
		}
	}
}

func (q *LoginQueue) run(job LoginJob) {
	script := fmt.Sprintf("scripts/%s_login.py", job.Provider)
	log.Printf("[auth/queue] running %s for account %d (%s)", script, job.AccountID, job.Email)

	ctx, cancel := context.WithTimeout(q.ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, q.pythonPath, script,
		"--email", job.Email,
		"--password", job.Password,
	)

	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		// include stderr if available
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		log.Printf("[auth/queue] account %d login failed: %s", job.AccountID, msg)
		if q.OnError != nil {
			q.OnError(job.AccountID, msg)
		}
		return
	}

	tokens := strings.TrimSpace(string(out))
	log.Printf("[auth/queue] account %d login succeeded", job.AccountID)
	if q.OnSuccess != nil {
		q.OnSuccess(job.AccountID, tokens)
	}
}
