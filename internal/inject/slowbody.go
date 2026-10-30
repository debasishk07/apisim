package inject

import (
	"io"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

// SlowBody streams the response body slowly at bytes_per_second.
type SlowBody struct {
	bytesPerSecond int
	trigger        Trigger
}

func NewSlowBody(bytesPerSecond int, when map[string]interface{}, rng *rand.Rand) (*SlowBody, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	return &SlowBody{bytesPerSecond: bytesPerSecond, trigger: trigger}, nil
}

func (s *SlowBody) Name() string { return "slow_body" }

func (s *SlowBody) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	return nil, nil
}

func (s *SlowBody) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	if !s.trigger.Fire(c, now) || s.bytesPerSecond <= 0 || resp.Body == nil {
		return resp, nil
	}
	resp.Body = &slowReader{src: resp.Body, rate: float64(s.bytesPerSecond)}
	return resp, nil
}

type slowReader struct {
	src   io.ReadCloser
	rate  float64
	mu    sync.Mutex
	last  time.Time
	credit float64
}

func (sr *slowReader) Read(p []byte) (int, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	now := time.Now()
	if !sr.last.IsZero() {
		sr.credit += sr.rate * now.Sub(sr.last).Seconds()
		if sr.credit > 1e6 {
			sr.credit = 1e6
		}
	}
	sr.last = now
	if sr.credit <= 0 {
		sr.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		sr.mu.Lock()
		if sr.credit <= 0 {
			sr.credit += sr.rate * 0.05
		}
	}
	n := int(sr.credit)
	if n > len(p) {
		n = len(p)
	}
	if n <= 0 {
		n = 1
	}
	sr.credit -= float64(n)
	nn, err := sr.src.Read(p[:n])
	return nn, err
}

func (sr *slowReader) Close() error { return sr.src.Close() }
