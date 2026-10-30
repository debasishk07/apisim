package inject

import (
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// AuthExpiry returns 401 once the route request counter passes a threshold,
// simulating an expired access token.
type AuthExpiry struct {
	status  int
	trigger Trigger
}

func NewAuthExpiry(afterRequests, status int, when map[string]interface{}, rng *rand.Rand) (*AuthExpiry, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	_ = afterRequests // threshold is encoded in the trigger (after_requests)
	return &AuthExpiry{status: status, trigger: trigger}, nil
}

func (a *AuthExpiry) Name() string { return "auth_expiry" }

func (a *AuthExpiry) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	if !a.trigger.Fire(c, now) {
		return nil, nil
	}
	resp := &http.Response{
		StatusCode: a.status,
		Status:     http.StatusText(a.status),
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"error":"auth expired"}`)),
		Request:    req,
	}
	return resp, nil
}

func (a *AuthExpiry) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	return resp, nil
}
