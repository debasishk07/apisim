package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"apisim/internal/config"
	"apisim/internal/clock"
	"apisim/internal/inject"
	"apisim/internal/match"
	"database/sql"
)

// Router routes incoming requests to matching routes or the default upstream.
type Router struct {
	scenario     *config.Scenario
	defaultProxy *httputil.ReverseProxy
	routes       []*routeEntry
	globalCtr    *inject.CounterRef
	sqlDB        *sql.DB
}

// routeEntry binds a matcher to its proxy behavior.
type routeEntry struct {
	matcher *match.Matcher
	mode    string
	handler http.Handler
}

// New builds a Router from a validated scenario.
func New(scenario *config.Scenario, db *sql.DB) (*Router, error) {
	r := &Router{scenario: scenario, globalCtr: inject.NewCounterRef(), sqlDB: db}
	if scenario.Defaults.Upstream != "" {
		u, err := url.Parse(scenario.Defaults.Upstream)
		if err != nil {
			return nil, fmt.Errorf("defaults.upstream %q: %w", scenario.Defaults.Upstream, err)
		}
		r.defaultProxy = newReverseProxy(u)
	}

	for _, rt := range scenario.Routes {
		entry, err := newRouteEntry(rt, scenario.Defaults, scenario.Seed, r.globalCtr, db)
		if err != nil {
			return nil, fmt.Errorf("route %q: %w", rt.Name, err)
		}
		r.routes = append(r.routes, entry)
	}
	return r, nil
}

func newRouteEntry(rt config.Route, defaults config.Defaults, seed int64, global *inject.CounterRef, db *sql.DB) (*routeEntry, error) {
	m := match.New(rt.Match.Method, rt.Match.Path, rt.Match.Headers, rt.Match.Query, rt.Match.BodyJSON)
	entry := &routeEntry{matcher: m, mode: rt.Mode}

	upstream := rt.Upstream
	if upstream == "" {
		upstream = defaults.Upstream
	}

	switch rt.Mode {
	case "proxy":
		if upstream == "" {
			return nil, fmt.Errorf("route %q: upstream is required for proxy mode", rt.Name)
		}
		u, err := url.Parse(upstream)
		if err != nil {
			return nil, fmt.Errorf("upstream %q: %w", upstream, err)
		}
		base := newReverseProxy(u)

		injects, err := inject.BuildInjectors(rt.Failures, seed)
		if err != nil {
			return nil, err
		}
		entry.handler = inject.NewPipeline(base, rt.Name, injects, seed, global, clock.Real())

	case "mock":
		entry.handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			writeMockResponse(w, rt.Response)
		})

	case "record":
		if upstream == "" {
			return nil, fmt.Errorf("route %q: upstream is required for record mode", rt.Name)
		}
		u, err := url.Parse(upstream)
		if err != nil {
			return nil, fmt.Errorf("upstream %q: %w", upstream, err)
		}
		base := newReverseProxy(u)

		injects, err := inject.BuildInjectors(rt.Failures, seed)
		if err != nil {
			return nil, err
		}
		entry.handler = inject.NewPipeline(base, rt.Name, injects, seed, global, clock.Real())

	case "replay":
		entry.handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			serveReplayResponse(w, req, db)
		})

	default:
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

// writeMockResponse writes a mock response based on the route's response config.
func writeMockResponse(w http.ResponseWriter, resp config.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.Status)
	if resp.Body != "" {
		_, _ = io.WriteString(w, resp.Body)
	} else {
		_, _ = io.WriteString(w, "{}")
	}
}

// serveReplayResponse serves a recorded response from the store.
func serveReplayResponse(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	if db == nil {
		writeJSONError(w, http.StatusInternalServerError, "no_store", "database not configured for replay mode")
		return
	}

	// Query the store for a matching recorded request
	method := req.Method
	path := req.URL.Path

	rows, err := db.Query(`
		SELECT r.status_code, r.method, r.path, GROUP_CONCAT(a.injector_type || ':' || a.details) as errors
		FROM requests r
		LEFT JOIN applied_failures a ON r.id = a.request_id
		WHERE r.method = ? AND r.path = ?
		GROUP BY r.id
		LIMIT 1
	`, method, path)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	if !rows.Next() {
		writeJSONError(w, http.StatusNotFound, "no_recorded_request", "no recorded response for this request")
		return
	}

	var statusCode int
	var storedMethod, storedPath, errorsStr string
	if err := rows.Scan(&statusCode, &storedMethod, &storedPath, &errorsStr); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if errorsStr != "" {
		// Include error info in response body
		body := fmt.Sprintf(`{"errors":[%q]}`, errorsStr)
		_, _ = io.WriteString(w, body)
	}
}

// Scenario returns the scenario configured for this router.
func (r *Router) Scenario() *config.Scenario {
	return r.scenario
}
