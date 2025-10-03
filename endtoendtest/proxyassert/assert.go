package proxyassert

import (
	"testing"
	"time"
)

func RequireNoCh[T any](t *testing.T, ch <-chan T, timeout time.Duration, message string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(message)
	case <-time.After(timeout):
	}
}

func RequireCh[T any](t *testing.T, ch chan T, timeout time.Duration, message string, args ...any) T {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(timeout):
		t.Fatalf(message, args...)
		// Never returned, but Go needs this
		var empty T
		return empty
	}
}
