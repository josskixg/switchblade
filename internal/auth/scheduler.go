package auth

import (
	"context"
	"log"
	"time"
)

// Scheduler runs WarmupQueue.RunOnce on a fixed interval.
type Scheduler struct {
	wq       *WarmupQueue
	interval time.Duration
	cancel   context.CancelFunc
	ctx      context.Context
}

// NewScheduler creates a Scheduler that fires every intervalMinutes minutes.
func NewScheduler(wq *WarmupQueue, intervalMinutes int) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		wq:       wq,
		interval: time.Duration(intervalMinutes) * time.Minute,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start launches the background ticker goroutine.
func (s *Scheduler) Start() {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		log.Printf("[auth/scheduler] warmup scheduled every %v", s.interval)
		for {
			select {
			case <-s.ctx.Done():
				log.Printf("[auth/scheduler] stopped")
				return
			case <-ticker.C:
				log.Printf("[auth/scheduler] running warmup")
				s.wq.RunOnce(s.ctx)
			}
		}
	}()
}

// Stop cancels the scheduler context, stopping the background goroutine.
func (s *Scheduler) Stop() {
	s.cancel()
}
