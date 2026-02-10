package proxy

import (
	"context"
	"errors"
)

type listenerWithCleanup struct {
	inner   Listener
	cleanup func() error
}

func (l *listenerWithCleanup) Start() error {
	return l.inner.Start()
}

func (l *listenerWithCleanup) Listen(ctx context.Context) error {
	return l.inner.Listen(ctx)
}

func (l *listenerWithCleanup) Stop() error {
	stopErr := l.inner.Stop()
	if l.cleanup == nil {
		return stopErr
	}
	cleanupErr := l.cleanup()
	return errors.Join(stopErr, cleanupErr)
}
