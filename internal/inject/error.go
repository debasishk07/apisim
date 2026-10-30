package inject

import (
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// Error returns a synthetic response (status/body/headers) without
// forwarding to the upstream.
type Error struct {
	status  int
	body    string
	headers map[string]string
	trigger Trigger
}

func NewError(status int, body string, headers map[string]string, when map[string]interface{}, rng *rand.Rand) (*Error, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	return &Error{
		status:  status,
		body:    body,
		headers: headers,
		trigger: trigger,
	}, nil
}

func (e *Error) Name() string { return "error" }

func (e *Error) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	if !e.trigger.Fire(c, now) {
		return nil, nil
	}
	resp := &http.Response{
		StatusCode: e.status,
		Status:     http.StatusText(e.status),
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(e.body)),
		Request:    req,
	}
	for k, v := range e.headers {
		resp.Header.Set(k, v)
	}
	return resp, nil
}

func (e *Error) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	return resp, nil
}
