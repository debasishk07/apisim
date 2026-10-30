package inject

import (
	"math/rand/v2"
	"net/http"
	"time"
)

// Latency delays the request before forwarding.
type Latency struct {
	delayMs  int
	jitterMs int
	trigger  Trigger
	rng      *rand.Rand
}

func NewLatency(delayMs, jitterMs int, when map[string]interface{}, rng *rand.Rand) (*Latency, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	return &Latency{
		delayMs:  delayMs,
		jitterMs: jitterMs,
		trigger:  trigger,
		rng:      rng,
	}, nil
}

func (l *Latency) Name() string { return "latency" }

func (l *Latency) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	if !l.trigger.Fire(c, now) {
		return nil, nil
	}
	delay := l.delayMs
	if l.jitterMs > 0 {
		delay += int(l.rng.Float64() * float64(l.jitterMs))
	}
	time.Sleep(time.Duration(delay) * time.Millisecond)
	return nil, nil
}

func (l *Latency) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	return resp, nil
}
