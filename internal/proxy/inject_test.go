package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"apisim/internal/config"

	"github.com/stretchr/testify/require"
)

func scenarioWithFails(upstream string, route config.Route) *config.Scenario {
	return &config.Scenario{
		Version: 1,
		Name:    "test",
		Listen:  ":0",
		Defaults: config.Defaults{Upstream: upstream},
		Routes:  []config.Route{route},
	}
}

func fail(typ string, when map[string]interface{}, cfg func(*config.Failure)) config.Failure {
	f := config.Failure{Type: typ, Trigger: when}
	if cfg != nil {
		cfg(&f)
	}
	return f
}

func TestInjectErrorIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer upstream.Close()

	scen := scenarioWithFails(upstream.URL, config.Route{
		Name:   "charge",
		Match:  config.Match{Method: "POST", Path: "/v1/charge"},
		Mode:   "proxy",
		Upstream: upstream.URL,
		Failures: []config.Failure{
			fail("error", map[string]interface{}{"probability": 1.0}, func(f *config.Failure) {
				f.Error = &config.Error{Status: 503, Body: `{"e":"down"}`, Headers: map[string]string{"Retry-After": "5"}}
			}),
		},
	})
	router, err := New(scen)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/charge", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 503, resp.StatusCode)
	require.Equal(t, "5", resp.Header.Get("Retry-After"))
	body, _ := io.ReadAll(resp.Body)
	require.JSONEq(t, `{"e":"down"}`, string(body))
}

func TestInjectLatencyIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	scen := scenarioWithFails(upstream.URL, config.Route{
		Name:   "charge",
		Match:  config.Match{Method: "GET", Path: "/v1/charge"},
		Mode:   "proxy",
		Upstream: upstream.URL,
		Failures: []config.Failure{
			fail("latency", map[string]interface{}{"probability": 1.0}, func(f *config.Failure) {
				f.Latency = &config.Latency{DelayMs: 80, JitterMs: 0}
			}),
		},
	})
	router, err := New(scen)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	defer srv.Close()

	start := time.Now()
	resp, err := http.Get(srv.URL + "/v1/charge")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.GreaterOrEqual(t, time.Since(start), 80*time.Millisecond)
}

func TestInjectRateLimitIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	scen := scenarioWithFails(upstream.URL, config.Route{
		Name:   "charge",
		Match:  config.Match{Method: "GET", Path: "/v1/charge"},
		Mode:   "proxy",
		Upstream: upstream.URL,
		Failures: []config.Failure{
			fail("rate_limit", map[string]interface{}{"probability": 1.0}, func(f *config.Failure) {
				f.RateLimit = &config.RateLimit{Limit: 1, WindowS: 60, Status: 429, Headers: map[string]string{"Retry-After": "30"}}
			}),
		},
	})
	router, err := New(scen)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	defer srv.Close()

	r1, _ := http.NewRequest("GET", srv.URL+"/v1/charge", nil)
	resp1, err := http.DefaultClient.Do(r1)
	require.NoError(t, err)
	resp1.Body.Close()
	require.Equal(t, 200, resp1.StatusCode)

	r2, _ := http.NewRequest("GET", srv.URL+"/v1/charge", nil)
	resp2, err := http.DefaultClient.Do(r2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, 429, resp2.StatusCode)
	require.Equal(t, "30", resp2.Header.Get("Retry-After"))
}

func TestInjectAuthExpiryIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	scen := scenarioWithFails(upstream.URL, config.Route{
		Name:   "charge",
		Match:  config.Match{Method: "GET", Path: "/v1/charge"},
		Mode:   "proxy",
		Upstream: upstream.URL,
		Failures: []config.Failure{
			fail("auth_expiry", map[string]interface{}{"after_requests": 2}, func(f *config.Failure) {
				f.AuthExpiry = &config.AuthExpiry{AfterRequests: 2, Status: 401}
			}),
		},
	})
	router, err := New(scen)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp1, err := http.Get(srv.URL + "/v1/charge")
	require.NoError(t, err)
	resp1.Body.Close()
	require.Equal(t, 200, resp1.StatusCode)

	resp2, err := http.Get(srv.URL + "/v1/charge")
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, 401, resp2.StatusCode)
}

func TestInjectMutateBodyIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc","amount":5000}`))
	}))
	defer upstream.Close()

	scen := scenarioWithFails(upstream.URL, config.Route{
		Name:   "charge",
		Match:  config.Match{Method: "GET", Path: "/v1/charge"},
		Mode:   "proxy",
		Upstream: upstream.URL,
		Failures: []config.Failure{
			fail("mutate_body", map[string]interface{}{"probability": 1.0}, func(f *config.Failure) {
				f.MutateBody = &config.MutateBody{Operations: []config.MutateOp{{Op: "remove_field", Path: "$.id"}}}
			}),
		},
	})
	router, err := New(scen)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/charge")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.JSONEq(t, `{"amount":5000}`, string(body))
}

func TestInjectDropConnectionIntegration(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	scen := scenarioWithFails(upstream.URL, config.Route{
		Name:   "charge",
		Match:  config.Match{Method: "GET", Path: "/v1/charge"},
		Mode:   "proxy",
		Upstream: upstream.URL,
		Failures: []config.Failure{
			fail("drop_connection", map[string]interface{}{"probability": 1.0}, nil),
		},
	})
	router, err := New(scen)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	defer srv.Close()

	client := &http.Transport{}
	tr := &http.Client{Transport: client}
	req, _ := http.NewRequest("GET", srv.URL+"/v1/charge", nil)
	_, err := client.Do(req)
	require.Error(t, err)

}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// never respond
		select {}
	}))
	defer upstream.Close()

	scen := scenarioWithFails(upstream.URL, config.Route{
		Name:   "charge",
		Match:  config.Match{Method: "GET", Path: "/v1/charge"},
		Mode:   "proxy",
		Upstream: upstream.URL,
		Failures: []config.Failure{
			fail("timeout", map[string]interface{}{"probability": 1.0}, nil),
		},
	})
	router, err := New(scen)
	require.NoError(t, err)
	srv := httptest.NewServer(router)
	defer srv.Close()

	client := &http.Client{Timeout: 60 * time.Millisecond}
	start := time.Now()
	resp, err := client.Get(srv.URL + "/v1/charge")
	require.Error(t, err)
	if resp != nil {
		resp.Body.Close()
	}
	require.Less(t, time.Since(start), 5*time.Second)
}
