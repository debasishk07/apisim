package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadValidScenario(t *testing.T) {
	data := []byte(`
version: 1
name: "checkout"
listen: ":8080"
seed: 42
defaults:
  upstream: https://api.stripe.com
  forward_headers: true
routes:
  - name: create-charge
    match:
      method: POST
      path: /v1/charges
    mode: proxy
    upstream: https://api.stripe.com
`)
	scen, err := Load(data)
	require.NoError(t, err)
	require.Equal(t, 1, scen.Version)
	require.Equal(t, "checkout", scen.Name)
	require.Equal(t, ":8080", scen.Listen)
	require.Equal(t, int64(42), scen.Seed)
	require.Equal(t, "https://api.stripe.com", scen.Defaults.Upstream)
	require.True(t, scen.Defaults.ForwardHeader)
	require.Len(t, scen.Routes, 1)
	require.Equal(t, "create-charge", scen.Routes[0].Name)
	require.Equal(t, "POST", scen.Routes[0].Match.Method)
	require.Equal(t, "/v1/charges", scen.Routes[0].Match.Path)
	require.Equal(t, "proxy", scen.Routes[0].Mode)
}

func TestLoadUnknownFieldReportsLine(t *testing.T) {
	data := []byte(`version: 1
name: "x"
routes:
  - name: r
    match:
      method: GET
      path: /a
    mode: proxy
    failures:
      - type: latncy   # typo
        delay_ms: 100
`)
	_, err := Load(data)
	require.Error(t, err)
	msg := err.Error()
	t.Logf("error: %v", err)
	require.Contains(t, msg, "latncy")
	require.Contains(t, msg, "did you mean")
}

func TestLoadBadVersion(t *testing.T) {
	data := []byte(`version: 2
name: "x"
routes:
  - name: r
    match:
      method: GET
      path: /a
`)
	_, err := Load(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported version")
}

func TestLoadUnknownTopLevelField(t *testing.T) {
	data := []byte(`version: 1
bogus: 1
routes:
  - name: r
    match:
      method: GET
      path: /a
`)
	_, err := Load(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bogus")
}

func TestMultiErrorFormat(t *testing.T) {
	errs := MultiError{
		&ValidationError{Path: "routes[0].type", Line: 5, Column: 3, Msg: "unknown type \"latncy\""},
	}
	require.Contains(t, errs.Error(), "line 5")
	require.Contains(t, errs.Error(), "col 3")
}

func TestDidYouMean(t *testing.T) {
	require.Equal(t, ", did you mean \"latency\"?", didYouMean("latncy", []string{"latency", "error"}))
	require.Equal(t, "", didYouMean("zzzzz", []string{"latency", "error"}))
}

func TestStringMapUnquote(t *testing.T) {
	require.Equal(t, `hello`, unquote(`"hello"`))
	require.Equal(t, `hello`, unquote(`'hello'`))
	require.Equal(t, `hello`, unquote(`hello`))
}

func TestLevenshtein(t *testing.T) {
	require.Equal(t, 1, levenshtein("latncy", "latency"))
	require.Equal(t, 0, levenshtein("latency", "latency"))
}

var _ = strings.Contains
