package inject

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
	"time"

	"apisim/internal/clock"
)

// Injector applies a failure to a request or response.
//
// BeforeRequest is called before the request is forwarded. If it returns a
// non-nil response, the request is short-circuited (not forwarded) and that
// response is returned to the client. AfterResponse is called on the
// (possibly upstream) response before it is sent to the client. Each injector
// self-gates using its own trigger.
type Injector interface {
	Name() string
	// BeforeRequest is called before forwarding. A non-nil response
	// short-circuits the request. A response with StatusCode == 0 means the
	// connection was hijacked/dropped; nothing further is written.
	BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error)
	AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error)
}

// Trigger decides whether an injector fires on a given request.
type Trigger interface {
	Fire(c Counters, now time.Time) bool
}

// Counters track how many requests have been seen.
type Counters struct {
	Route  int64
	Global int64
}

// counter is a thread-safe incrementing counter.
type counter struct {
	v int64
}

func (c *counter) inc() int64 { return atomic.AddInt64(&c.v, 1) }

// CounterRef exposes an incrementing counter for cross-package use.
type CounterRef struct {
	c *counter
}

func NewCounterRef() *CounterRef { return &CounterRef{c: &counter{}} }

func (r *CounterRef) Inc() int64 { return r.c.inc() }

// ---------------------------------------------------------------- trigger engine

type probTrigger struct {
	p   float64
	rng *rand.Rand
}

func (t *probTrigger) Fire(c Counters, now time.Time) bool { return t.rng.Float64() < t.p }

type everyNthTrigger struct{ n int64 }

func (t *everyNthTrigger) Fire(c Counters, now time.Time) bool {
	return t.n > 0 && c.Route%t.n == 0 && c.Route > 0
}

type afterRequestsTrigger struct{ n int64 }

func (t *afterRequestsTrigger) Fire(c Counters, now time.Time) bool {
	return t.n > 0 && c.Route >= t.n
}

type firstNRequestsTrigger struct{ n int64 }

func (t *firstNRequestsTrigger) Fire(c Counters, now time.Time) bool {
	return t.n > 0 && c.Route <= t.n
}

type betweenRequestsTrigger struct{ lo, hi int64 }

func (t *betweenRequestsTrigger) Fire(c Counters, now time.Time) bool {
	return c.Route >= t.lo && c.Route <= t.hi
}

type timeWindowTrigger struct {
	start time.Time
	dur   time.Duration
}

func (t *timeWindowTrigger) Fire(c Counters, now time.Time) bool {
	return !now.Before(t.start) && now.Before(t.start.Add(t.dur))
}

type andTrigger []Trigger

func (t andTrigger) Fire(c Counters, now time.Time) bool {
	for _, sub := range t {
		if !sub.Fire(c, now) {
			return false
		}
	}
	return true
}

type alwaysTrigger struct{}

func (alwaysTrigger) Fire(c Counters, now time.Time) bool { return true }

// ParseTrigger builds a Trigger from a when map. rng is shared so the
// probability sequence is deterministic given a seed.
func ParseTrigger(when map[string]interface{}, rng *rand.Rand) (Trigger, error) {
	var subs []Trigger
	for k, v := range when {
		sub, err := parseTriggerOne(k, v, rng)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if len(subs) == 0 {
		return alwaysTrigger{}, nil
	}
	if len(subs) == 1 {
		return subs[0], nil
	}
	return andTrigger(subs), nil
}

func parseTriggerOne(k string, v interface{}, rng *rand.Rand) (Trigger, error) {
	switch k {
	case "probability":
		p, ok := toFloat(v)
		if !ok {
			return nil, errors.New("probability must be a number")
		}
		if p < 0 || p > 1 {
			return nil, errors.New("probability must be between 0 and 1")
		}
		return &probTrigger{p: p, rng: rng}, nil
	case "every_nth":
		n, ok := toFloat(v)
		if !ok || n < 1 {
			return nil, errors.New("every_nth must be a positive integer")
		}
		return &everyNthTrigger{n: int64(n)}, nil
	case "after_requests":
		n, ok := toFloat(v)
		if !ok || n < 1 {
			return nil, errors.New("after_requests must be a positive integer")
		}
		return &afterRequestsTrigger{n: int64(n)}, nil
	case "first_n_requests":
		n, ok := toFloat(v)
		if !ok || n < 1 {
			return nil, errors.New("first_n_requests must be a positive integer")
		}
		return &firstNRequestsTrigger{n: int64(n)}, nil
	case "between_requests":
		arr, ok := v.([]interface{})
		if !ok || len(arr) != 2 {
			return nil, errors.New("between_requests must be [a, b]")
		}
		lo, ok := toFloat(arr[0])
		hi, ok2 := toFloat(arr[1])
		if !ok || !ok2 || lo > hi {
			return nil, errors.New("between_requests must be [a, b] with a <= b")
		}
		return &betweenRequestsTrigger{lo: int64(lo), hi: int64(hi)}, nil
	case "time_window":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("time_window must be a duration string")
		}
		dur, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("time_window %q: %w", s, err)
		}
		return &timeWindowTrigger{dur: dur}, nil
	default:
		return nil, fmt.Errorf("unknown trigger %q", k)
	}
}

// toFloat converts a decoded YAML/JSON number (float64, int, int64) to float64.
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

// ---------------------------------------------------------------- pipeline

// Pipeline chains injectors around an inner handler.
type Pipeline struct {
	inner   http.Handler
	injects []Injector

	rng        *rand.Rand
	clk        clock.Clock
	routeCtr   *counter
	globalCtr  *CounterRef
	routeName  string
	startTime  time.Time
}

// NewPipeline builds a Pipeline. injects are applied in order.
func NewPipeline(inner http.Handler, routeName string, injects []Injector, seed int64, global *CounterRef, clk clock.Clock) *Pipeline {
	if clk == nil {
		clk = &realClock{}
	}
	return &Pipeline{
		inner:     inner,
		injects:   injects,
		rng:       rand.New(rand.NewPCG(uint64(seed), uint64(seed))),
		clk:       clk,
		routeCtr:  &counter{},
		globalCtr: global,
		routeName: routeName,
		startTime: clk.Now(),
	}
}

func (p *Pipeline) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := p.routeCtr.inc()
	gc := p.globalCtr.Inc()
	counters := Counters{Route: rc, Global: gc}
	now := p.clk.Now()

	// BeforeRequest phase: short-circuit possible.
	for _, inj := range p.injects {
		resp, err := inj.BeforeRequest(w, r, counters, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if resp != nil {
			if resp.StatusCode == 0 {
				// Connection hijacked/dropped; nothing further to write.
				return
			}
			writeResponse(w, resp)
			return
		}
	}

	// Forward and capture the response.
	rec := &responseCapture{hdr: make(http.Header), Code: 200}
	p.inner.ServeHTTP(rec, r)

	resp := &http.Response{
		StatusCode: rec.Code,
		Status:     http.StatusText(rec.Code),
		Header:     rec.Header(),
		Body:       io.NopCloser(bytes.NewReader(rec.Body)),
		Request:    r,
	}

	// AfterResponse phase.
	for _, inj := range p.injects {
		out, err := inj.AfterResponse(resp, counters, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if out == nil {
			out = &http.Response{StatusCode: http.StatusInternalServerError}
		}
		resp = out
	}

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		_, _ = io.Copy(w, resp.Body)
		_ = resp.Body.Close()
	}
}

type responseCapture struct {
	http.ResponseWriter
	Code  int
	hdr   http.Header
	Body  []byte
}

func (rc *responseCapture) Header() http.Header { return rc.hdr }
func (rc *responseCapture) WriteHeader(code int) { rc.Code = code }
func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.Body = append(rc.Body, b...)
	return len(b), nil
}

func writeResponse(w http.ResponseWriter, resp *http.Response) {
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		_, _ = io.Copy(w, resp.Body)
		_ = resp.Body.Close()
	}
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// ---------------------------------------------------------------- clock

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }

var _ clock.Clock = realClock{}
