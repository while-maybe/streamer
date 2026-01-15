package media

import "context"

type IOLimiter struct {
	sem chan struct{} //acts as a semaphore
}

func NewIOLimiter(maxConcurrent int) *IOLimiter {
	return &IOLimiter{sem: make(chan struct{}, maxConcurrent)}
}

// Acquire blocks until a slot is free OR context is cancelled
func (i *IOLimiter) TryAcquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case i.sem <- struct{}{}:
		return nil
	}
}

func (i *IOLimiter) Release() {
	<-i.sem
}
