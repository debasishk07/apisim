package match

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func req(method, path string, hdr http.Header, query url.Values, body map[string]interface{}) *Request {
	return &Request{Method: method, Path: path, Header: hdr, Query: query, BodyJSON: body}
}

func TestExactMatch(t *testing.T) {
	m := New("GET", "/v1/charges", nil, nil, nil)
	res := m.Match(req("GET", "/v1/charges", nil, nil, nil))
	require.True(t, res.Matched)
	require.Equal(t, 4+2, res.Specificity) // exact path + method
}

func TestMethodMismatch(t *testing.T) {
	m := New("POST", "/v1/charges", nil, nil, nil)
	res := m.Match(req("GET", "/v1/charges", nil, nil, nil))
	require.False(t, res.Matched)
}

func TestPathParams(t *testing.T) {
	m := New("GET", "/v1/users/{id}", nil, nil, nil)
	res := m.Match(req("GET", "/v1/users/123", nil, nil, nil))
	require.True(t, res.Matched)
	require.Equal(t, "123", res.Params["id"])
	require.Equal(t, 3+2, res.Specificity) // param path + method
}

func TestWildcard(t *testing.T) {
	m := New("GET", "/v1/*", nil, nil, nil)
	res := m.Match(req("GET", "/v1/charges/123", nil, nil, nil))
	require.True(t, res.Matched)
	res2 := m.Match(req("GET", "/other/x", nil, nil, nil))
	require.False(t, res2.Matched)
}

func TestHeadersMatch(t *testing.T) {
	m := New("GET", "/v1/charges", map[string]string{"X-Env": "test"}, nil, nil)
	require.True(t, m.Match(req("GET", "/v1/charges", http.Header{"X-Env": {"test"}}, nil, nil)).Matched)
	require.False(t, m.Match(req("GET", "/v1/charges", http.Header{"X-Env": {"prod"}}, nil, nil)).Matched)
}

func TestQueryMatch(t *testing.T) {
	m := New("GET", "/v1/charges", nil, map[string]string{"currency": "usd"}, nil)
	require.True(t, m.Match(req("GET", "/v1/charges", nil, url.Values{"currency": {"usd"}}, nil)).Matched)
	require.False(t, m.Match(req("GET", "/v1/charges", nil, url.Values{"currency": {"eur"}}, nil)).Matched)
}

func TestBodyJSONMatch(t *testing.T) {
	m := New("POST", "/v1/charges", nil, nil, map[string]interface{}{"$.amount": 5000})
	require.True(t, m.Match(req("POST", "/v1/charges", nil, nil, map[string]interface{}{"$.amount": 5000})).Matched)
	require.False(t, m.Match(req("POST", "/v1/charges", nil, nil, map[string]interface{}{"$.amount": 100})).Matched)
}

func TestSpecificityExactBeatsParam(t *testing.T) {
	exact := New("GET", "/v1/charges", nil, nil, nil)
	param := New("GET", "/v1/{id}", nil, nil, nil)
	require.Greater(t, exact.specificity(), param.specificity())
}

func TestSpecificityMoreConstraintsWin(t *testing.T) {
	a := New("GET", "/v1/charges", nil, nil, nil)
	b := New("GET", "/v1/charges", map[string]string{"X": "y"}, nil, nil)
	require.Greater(t, b.specificity(), a.specificity())
}
