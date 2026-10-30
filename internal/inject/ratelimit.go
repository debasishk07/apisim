package inject

import (
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

// RateLimit implements a token bucket: `limit` requests per `window_s`
// seconds. When a bucket is empty it returns 429 (configurable).
type RateLimit struct {
	limit    int
	windowS  int
	status   int
	headers  map[string]string
	trigger  Trigger
	mu       sync.Mutex
	tokens   float64
	last     time.Time
	ratePerS float64
}

func NewRateLimit(limit, windowS, status int, headers map[string]string, when map[string]interface{}, rng *rand.Rand) (*RateLimit, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	return &RateLimit{
		limit:    limit,
		windowS:  windowS,
		status:   status,
		headers:  headers,
		trigger:  trigger,
		tokens:   float64(limit),
		last:     time.Now(),
		ratePerS: float64(limit) / float64(windowS),
	}, nil
}

func (r *RateLimit) Name() string { return "rate_limit" }

func (r *RateLimit) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	if !r.trigger.Fire(c, now) {
		return nil, nil
	}
	r.mu.Lock()
	elapsed := now.Sub(r.last).Seconds()
	r.tokens += elapsed * r.ratePerS
	if r.tokens > float64(r.limit) {
		r.tokens = float64(r.limit)
	}
	r.last = now
	if r.tokens < 1.0 {
		r.tokens = 0
		r.mu.Unlock()
		return r.rateLimited(req), nil
	}
	r.tokens--
	r.mu.Unlock()
	return nil, nil
}

func (r *RateLimit) rateLimited(req *http.Request) *http.Response {
	resp := &http.Response{
		StatusCode: r.status,
		Status:     http.StatusText(r.status),
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}
	for k, v := range r.headers {
		resp.Header.Set(k, v)
	}
	return resp
}

func (r *RateLimit) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	return resp, nil
}
