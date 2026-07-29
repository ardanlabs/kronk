package session

import "context"

// Nop returns a disabled observer that performs no allocation, storage, or
// background work.
func Nop() Observer {
	return nopObserver{}
}

type nopObserver struct{}

func (nopObserver) RequestStarted(RequestStart) error {
	return nil
}

func (nopObserver) RequestCompleted(context.Context, RequestCompletion) error {
	return nil
}

func (nopObserver) SessionCompleted(context.Context, Key) error {
	return nil
}
