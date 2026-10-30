package inject

import (
	"math/rand/v2"
	"net/http"
	"time"
)

// Timeout never responds until the client gives up (context canceled).
type Timeout struct {
	trigger Trigger
}

func NewTimeout(when map[string]interface{}, rng *rand.Rand) (*Timeout, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	return &Timeout{trigger: trigger}, nil
}

func (t *Timeout) Name() string { return "timeout" }

func (t *Timeout) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	if !t.trigger.Fire(c, now) {
		return nil, nil
	}
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func (t *Timeout) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	return resp, nil
}
