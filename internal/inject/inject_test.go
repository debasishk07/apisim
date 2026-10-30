package inject

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------- trigger tests

func TestTriggerEveryNth(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	trig, err := ParseTrigger(map[string]interface{}{"every_nth": 3}, rng)
	require.NoError(t, err)
	require.False(t, trig.Fire(Counters{Route: 1}, time.Time{}))
	require.False(t, trig.Fire(Counters{Route: 2}, time.Time{}))
	require.True(t, trig.Fire(Counters{Route: 3}, time.Time{}))
	require.False(t, trig.Fire(Counters{Route: 4}, time.Time{}))
	require.True(t, trig.Fire(Counters{Route: 6}, time.Time{}))
}

func TestTriggerAfterRequests(t *testing.T) {
	trig, err := ParseTrigger(map[string]interface{}{"after_requests": 5}, rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.False(t, trig.Fire(Counters{Route: 4}, time.Time{}))
	require.True(t, trig.Fire(Counters{Route: 5}, time.Time{}))
	require.True(t, trig.Fire(Counters{Route: 100}, time.Time{}))
}

func TestTriggerFirstN(t *testing.T) {
	trig, err := ParseTrigger(map[string]interface{}{"first_n_requests": 2}, rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.True(t, trig.Fire(Counters{Route: 1}, time.Time{}))
	require.True(t, trig.Fire(Counters{Route: 2}, time.Time{}))
	require.False(t, trig.Fire(Counters{Route: 3}, time.Time{}))
}

func TestTriggerBetween(t *testing.T) {
	trig, err := ParseTrigger(map[string]interface{}{"between_requests": []interface{}{3, 5}}, rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.False(t, trig.Fire(Counters{Route: 2}, time.Time{}))
	require.True(t, trig.Fire(Counters{Route: 3}, time.Time{}))
	require.True(t, trig.Fire(Counters{Route: 5}, time.Time{}))
	require.False(t, trig.Fire(Counters{Route: 6}, time.Time{}))
}

func TestTriggerTimeWindow(t *testing.T) {
	start := time.Now()
	trig, err := ParseTrigger(map[string]interface{}{"time_window": "10s"}, rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	tw := trig.(*timeWindowTrigger)
	tw.start = start
	require.True(t, tw.Fire(Counters{}, start))
	require.True(t, tw.Fire(Counters{}, start.Add(5*time.Second)))
	require.False(t, tw.Fire(Counters{}, start.Add(15*time.Second)))
}

func TestTriggerAND(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	// probability(0) AND every_nth(2) -> never
	trig, err := ParseTrigger(map[string]interface{}{
		"probability": 0.0,
		"every_nth":   2,
	}, rng)
	require.NoError(t, err)
	require.False(t, trig.Fire(Counters{Route: 2}, time.Time{}))

	rng2 := rand.New(rand.NewPCG(1, 1))
	trig2, err := ParseTrigger(map[string]interface{}{
		"probability": 1.0,
		"every_nth":   2,
	}, rng2)
	require.NoError(t, err)
	require.False(t, trig2.Fire(Counters{Route: 1}, time.Time{}))
	require.True(t, trig2.Fire(Counters{Route: 2}, time.Time{}))
	require.False(t, trig2.Fire(Counters{Route: 3}, time.Time{}))
	require.True(t, trig2.Fire(Counters{Route: 4}, time.Time{}))
}

// ---------------------------------------------------------------- injector unit tests

func alwaysWhen() map[string]interface{} {
	return map[string]interface{}{"probability": 1.0}
}

func TestErrorInjector(t *testing.T) {
	inj, err := NewError(503, `{"e":"down"}`, map[string]string{"Retry-After": "5"}, alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.Equal(t, "error", inj.Name())

	resp, err := inj.BeforeRequest(nil, nil, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 503, resp.StatusCode)
	require.Equal(t, "5", resp.Header.Get("Retry-After"))
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, `{"e":"down"}`, string(body))

	// not firing when trigger doesn't match (prob 0)
	inj2, _ := NewError(503, "x", nil, map[string]interface{}{"probability": 0.0}, rand.New(rand.NewPCG(1, 1)))
	resp2, err := inj2.BeforeRequest(nil, nil, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	require.Nil(t, resp2)
}

func TestLatencyInjector(t *testing.T) {
	inj, err := NewLatency(60, 0, alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.Equal(t, "latency", inj.Name())

	start := time.Now()
	resp, err := inj.BeforeRequest(nil, nil, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	require.Nil(t, resp)
	require.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond)

	// not firing
	inj2, _ := NewLatency(60, 0, map[string]interface{}{"probability": 0.0}, rand.New(rand.NewPCG(1, 1)))
	start = time.Now()
	resp2, err := inj2.BeforeRequest(nil, nil, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	require.Nil(t, resp2)
	require.Less(t, time.Since(start), 10*time.Millisecond)
}

func TestRateLimitInjector(t *testing.T) {
	inj, err := NewRateLimit(1, 60, 429, map[string]string{"Retry-After": "30"}, alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.Equal(t, "rate_limit", inj.Name())

	now := time.Now()
	// first request: token available -> pass
	resp, err := inj.BeforeRequest(nil, nil, Counters{Route: 1}, now)
	require.NoError(t, err)
	require.Nil(t, resp)
	// second request immediately: no token -> 429
	resp2, err := inj.BeforeRequest(nil, nil, Counters{Route: 2}, now)
	require.NoError(t, err)
	require.NotNil(t, resp2)
	require.Equal(t, 429, resp2.StatusCode)
	require.Equal(t, "30", resp2.Header.Get("Retry-After"))
}

func TestAuthExpiryInjector(t *testing.T) {
	inj, err := NewAuthExpiry(3, 401, map[string]interface{}{"after_requests": 3}, rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.Equal(t, "auth_expiry", inj.Name())

	now := time.Now()
	require.NoError(t, err)
	resp1, _ := inj.BeforeRequest(nil, nil, Counters{Route: 1}, now)
	require.Nil(t, resp1)
	resp2, _ := inj.BeforeRequest(nil, nil, Counters{Route: 2}, now)
	require.Nil(t, resp2)
	resp3, err := inj.BeforeRequest(nil, nil, Counters{Route: 3}, now)
	require.NoError(t, err)
	require.NotNil(t, resp3)
	require.Equal(t, 401, resp3.StatusCode)
}

func TestDropConnInjector(t *testing.T) {
	inj, err := NewDropConn(alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.Equal(t, "drop_connection", inj.Name())

	resp, err := inj.BeforeRequest(nil, nil, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 0, resp.StatusCode) // signals connection dropped
}

func TestTimeoutInjector(t *testing.T) {
	inj, err := NewTimeout(alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.Equal(t, "timeout", inj.Name())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("GET", "http://example.com/x", nil)
	req = req.WithContext(ctx)
	start := time.Now()
	_, err = inj.BeforeRequest(nil, req, Counters{Route: 1}, time.Time{})
	require.Error(t, err)
	require.Less(t, time.Since(start), 2*time.Second) // should return quickly on cancel
}

func TestMutateBodyInjector(t *testing.T) {
	inj, err := NewMutateBody([]MutateOp{{Op: "remove_field", Path: "$.id"}}, alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	require.Equal(t, "mutate_body", inj.Name())

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"abc","amount":5000}`)),
	}
	out, err := inj.AfterResponse(resp, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	body, _ := io.ReadAll(out.Body)
	require.JSONEq(t, `{"amount":5000}`, string(body))
}

func TestMutateBodyRename(t *testing.T) {
	inj, err := NewMutateBody([]MutateOp{{Op: "rename_field", Path: "$.amount", To: "$.total"}}, alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"amount":5000}`)),
	}
	out, err := inj.AfterResponse(resp, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	body, _ := io.ReadAll(out.Body)
	require.JSONEq(t, `{"total":5000}`, string(body))
}

func TestMutateBodyTruncate(t *testing.T) {
	inj, err := NewMutateBody([]MutateOp{{Op: "truncate", AtBytes: 20}}, alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"id":"abc","amount":5000}`)),
	}
	out, err := inj.AfterResponse(resp, Counters{Route: 1}, time.Time{})
	require.NoError(t, err)
	body, _ := io.ReadAll(out.Body)
	require.Len(t, body, 20)
}

// ---------------------------------------------------------------- seeded determinism

func TestSeededDeterminism(t *testing.T) {
	run := func(seed int64) []bool {
		var fired []bool
		rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)))
		trig, _ := ParseTrigger(map[string]interface{}{"probability": 0.3}, rng)
		for i := int64(1); i <= 10; i++ {
			fired = append(fired, trig.Fire(Counters{Route: i}, time.Time{}))
		}
		return fired
	}
	require.Equal(t, run(42), run(42))
	require.NotEqual(t, run(42), run(99))
}

// ---------------------------------------------------------------- pipeline integration

func TestPipelineErrorShortCircuits(t *testing.T) {
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("upstream"))
	})

	inj, _ := NewError(503, "down", nil, alwaysWhen(), rand.New(rand.NewPCG(1, 1)))
	pipe := NewPipeline(base, "", []Injector{inj}, 1, NewCounterRef(), nil)

	req := httptest.NewRequest("GET", "http://example.com/x", nil)
	rec := httptest.NewRecorder()
	pipe.ServeHTTP(rec, req)
	require.Equal(t, 503, rec.Code)
	require.Equal(t, "down", rec.Body.String())
}
