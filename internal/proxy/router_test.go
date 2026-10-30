package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisim/internal/config"
	"github.com/stretchr/testify/require"
)

func scenario(upstream string) *config.Scenario {
	return &config.Scenario{
		Version:  1,
		Name:     "test",
		Listen:   ":0",
		Defaults: config.Defaults{Upstream: upstream},
		Routes: []config.Route{
			{
				Name:  "get",
				Match: config.Match{Method: "GET", Path: "/v1/get"},
				Mode:  "proxy",
			},
			{
				Name:     "post",
				Match:    config.Match{Method: "POST", Path: "/v1/post"},
				Mode:     "proxy",
				Upstream: upstream,
			},
		},
	}
}

func TestProxyForwardsGET(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "yes")
		_, _ = w.Write([]byte("get-ok"))
	}))
	defer upstream.Close()

	scen := scenario(upstream.URL)
	router, err := New(scen)
	require.NoError(t, err)

	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/get?currency=usd")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "yes", resp.Header.Get("X-Upstream"))
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, "get-ok", string(body))
}

func TestProxyForwardsPOSTBodyAndHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"echo":` + string(body) + `}`))
	}))
	defer upstream.Close()

	scen := scenario(upstream.URL)
	router, err := New(scen)
	require.NoError(t, err)

	srv := httptest.NewServer(router)
	defer srv.Close()

	reqBody := `{"amount":5000,"currency":"usd"}`
	resp, err := http.Post(srv.URL+"/v1/post", "application/json", strings.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.JSONEq(t, `{"echo":{"amount":5000,"currency":"usd"}}`, string(body))
}

func TestProxyForwardsHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Env")))
	}))
	defer upstream.Close()

	scen := scenario(upstream.URL)
	router, err := New(scen)
	require.NoError(t, err)

	srv := httptest.NewServer(router)
	defer srv.Close()

	req, err := http.NewRequest("GET", srv.URL+"/v1/get", nil)
	req.Header.Set("X-Env", "test")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, "test", string(body))
}

func TestUnmatchedReturns404JSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("upstream"))
	}))
	defer upstream.Close()

	scen := &config.Scenario{
		Version: 1,
		Name:    "test",
		Listen:  ":0",
		Routes: []config.Route{
			{
				Name:  "get",
				Match: config.Match{Method: "GET", Path: "/v1/get"},
				Mode:  "proxy",
			},
		},
	}
	router, err := New(scen)
	require.NoError(t, err)

	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/nope")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 404, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(resp.Body)
	require.JSONEq(t, `{"error":{"code":"no_route_matched","message":"no route matched this request; pass-through is disabled"}}`, string(body))
}
