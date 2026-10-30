package clock

import (
	"sync"
	"time"
)

// Clock is an injectable time source for deterministic tests.
type Clock interface {
	Now() time.Time
	Since(time.Time) time.Duration
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }

// Real returns the real, non-injectable clock.
func Real() Clock { return realClock{} }

// Mock is a manually-advanceable clock for tests.
type Mock struct {
	mu   *sync.Mutex
	now  time.Time
}

func NewMock(now time.Time) *Mock {
	return &Mock{mu: &sync.Mutex{}, now: now}
}

func (m *Mock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *Mock) Since(t time.Time) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now.Sub(t)
}

func (m *Mock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}

func (m *Mock) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = t
}

var _ Clock = (*Mock)(nil)
