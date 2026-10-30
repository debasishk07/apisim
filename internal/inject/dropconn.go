package inject

import (
	"math/rand/v2"
	"net/http"
	"time"
)

// DropConn closes the TCP connection abruptly via http.Hijacker.
type DropConn struct {
	trigger Trigger
}

func NewDropConn(when map[string]interface{}, rng *rand.Rand) (*DropConn, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	return &DropConn{trigger: trigger}, nil
}

func (d *DropConn) Name() string { return "drop_connection" }

func (d *DropConn) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	if !d.trigger.Fire(c, now) {
		return nil, nil
	}
	if hj, ok := w.(http.Hijacker); ok {
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}
	// StatusCode 0 signals the pipeline that the connection was dropped.
	return &http.Response{}, nil
}

func (d *DropConn) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	return resp, nil
}
