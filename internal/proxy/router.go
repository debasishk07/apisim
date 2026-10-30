package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"apisim/internal/config"
	"apisim/internal/inject"
	"apisim/internal/match"
)

// Router routes incoming requests to matching routes or the default upstream.
type Router struct {
	scenario     *config.Scenario
	defaultProxy *httputil.ReverseProxy
	routes       []*routeEntry
	globalCtr    *inject.CounterRef
}

// routeEntry binds a matcher to its proxy behavior.
type routeEntry struct {
	matcher *match.Matcher
	mode    string
	handler http.Handler
}

// New builds a Router from a validated scenario.
func New(scenario *config.Scenario) (*Router, error) {
	r := &Router{scenario: scenario, globalCtr: inject.NewCounterRef()}
	if scenario.Defaults.Upstream != "" {
		u, err := url.Parse(scenario.Defaults.Upstream)
		if err != nil {
			return nil, fmt.Errorf("defaults.upstream %q: %w", scenario.Defaults.Upstream, err)
		}
		r.defaultProxy = newReverseProxy(u)
	}

	for _, rt := range scenario.Routes {
		entry, err := newRouteEntry(rt, scenario.Defaults, scenario.Seed, r.globalCtr)
		if err != nil {
			return nil, fmt.Errorf("route %q: %w", rt.Name, err)
		}
		r.routes = append(r.routes, entry)
	}
	return r, nil
}

func newRouteEntry(rt config.Route, defaults config.Defaults, seed int64, global *inject.CounterRef) (*routeEntry, error) {
	m := match.New(rt.Match.Method, rt.Match.Path, rt.Match.Headers, rt.Match.Query, rt.Match.BodyJSON)
	entry := &routeEntry{matcher: m, mode: rt.Mode}

	upstream := rt.Upstream
	if upstream == "" {
		upstream = defaults.Upstream
	}
	if upstream != "" && rt.Mode == "proxy" {
		u, err := url.Parse(upstream)
		if err != nil {
			return nil, fmt.Errorf("upstream %q: %w", upstream, err)
		}
		base := newReverseProxy(u)

		injects, err := inject.BuildInjectors(rt.Failures, seed)
		if err != nil {
			return nil, err
		}
		entry.handler = inject.NewPipeline(base, rt.Name, injects, seed, global, nil)
	} else {
		entry.handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			writeJSONError(w, http.StatusNotImplemented, "mode_not_implemented", fmt.Sprintf("route %q mode %q not yet implemented", m.Path, rt.Mode))
		})
	}
	return entry, nil
}

// ServeHTTP dispatches the request.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	mReq := &match.Request{
		Method: req.Method,
		Path:   req.URL.Path,
		Header: req.Header,
		Query:  req.URL.Query(),
	}

	var best *routeEntry
	var bestResult match.MatchResult
	bestResult.Matched = false

	for _, entry := range r.routes {
		res := entry.matcher.Match(mReq)
		if res.Matched {
			if !bestResult.Matched || res.Specificity > bestResult.Specificity {
				best = entry
				bestResult = res
			}
		}
	}

	if best == nil {
		if r.defaultProxy != nil {
			r.defaultProxy.ServeHTTP(w, req)
			return
		}
		writeJSONError(w, http.StatusNotFound, "no_route_matched", "no route matched this request; pass-through is disabled")
		return
	}

	best.handler.ServeHTTP(w, req)
}

// newReverseProxy builds a reverse proxy that forwards correctly.
func newReverseProxy(target *url.URL) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		removeHopByHop(req.Header)

		if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
				req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
			} else {
				req.Header.Set("X-Forwarded-For", clientIP)
			}
		}
		if req.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}

	return &httputil.ReverseProxy{
		Director: director,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{}).DialContext,
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		},
		ModifyResponse: func(resp *http.Response) error {
			removeHopByHop(resp.Header)
			return nil
		},
	}
}

// removeHopByHop strips headers that must not be forwarded.
func removeHopByHop(header http.Header) {
	for _, h := range hopByHop {
		header.Del(h)
	}
}

var hopByHop = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := fmt.Sprintf(`{"error":{"code":%q,"message":%q}}`, code, msg)
	_, _ = io.WriteString(w, body)
}
