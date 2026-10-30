package match

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// MatchResult holds whether a route matched and its specificity score.
type MatchResult struct {
	Matched     bool
	Specificity int
	Params      map[string]string
}

// Matcher matches requests against a route's match criteria.
type Matcher struct {
	Method   string
	Path     string
	Headers  map[string]string
	Query    map[string]string
	BodyJSON map[string]interface{}
}

// New builds a Matcher from route match config.
func New(method, path string, headers, query map[string]string, bodyJSON map[string]interface{}) *Matcher {
	return &Matcher{
		Method:   method,
		Path:     path,
		Headers:  headers,
		Query:    query,
		BodyJSON: bodyJSON,
	}
}

// Match evaluates the request against this matcher.
func (m *Matcher) Match(req *Request) MatchResult {
	if !m.matchMethod(req) {
		return MatchResult{Matched: false}
	}
	params, ok := m.matchPath(req.Path)
	if !ok {
		return MatchResult{Matched: false}
	}
	if !m.matchHeaders(req) {
		return MatchResult{Matched: false}
	}
	if !m.matchQuery(req) {
		return MatchResult{Matched: false}
	}
	if !m.matchBodyJSON(req) {
		return MatchResult{Matched: false}
	}
	return MatchResult{Matched: true, Specificity: m.specificity(), Params: params}
}

func (m *Matcher) matchMethod(req *Request) bool {
	if m.Method == "" {
		return true
	}
	return strings.EqualFold(m.Method, req.Method)
}

func (m *Matcher) matchPath(reqPath string) (map[string]string, bool) {
	if m.Path == "" || m.Path == "/" {
		return map[string]string{}, reqPath == "/" || m.Path == reqPath
	}
	patternSegs := splitPath(m.Path)
	reqSegs := splitPath(reqPath)

	if isWildcard(patternSegs) {
		prefix := "/" + strings.Join(patternSegs[:len(patternSegs)-1], "/")
		if strings.HasPrefix(reqPath, prefix) {
			return map[string]string{}, true
		}
		return nil, false
	}

	if len(patternSegs) != len(reqSegs) {
		return nil, false
	}
	params := map[string]string{}
	for i, ps := range patternSegs {
		rs := reqSegs[i]
		if isParam(ps) {
			name := ps[1 : len(ps)-1]
			params[name] = rs
		} else if ps != rs {
			return nil, false
		}
	}
	return params, true
}

func (m *Matcher) matchHeaders(req *Request) bool {
	for k, v := range m.Headers {
		if req.Header.Get(k) != v {
			return false
		}
	}
	return true
}

func (m *Matcher) matchQuery(req *Request) bool {
	for k, v := range m.Query {
		if req.Query.Get(k) != v {
			return false
		}
	}
	return true
}

func (m *Matcher) matchBodyJSON(req *Request) bool {
	if len(m.BodyJSON) == 0 {
		return true
	}
	if req.BodyJSON == nil {
		return false
	}
	for k, expected := range m.BodyJSON {
		actual, ok := req.BodyJSON[k]
		if !ok {
			return false
		}
		if !equalJSON(expected, actual) {
			return false
		}
	}
	return true
}

func (m *Matcher) specificity() int {
	s := 0
	if m.Path != "" {
		if isWildcard(splitPath(m.Path)) {
			s += 2
		} else if hasParam(splitPath(m.Path)) {
			s += 3
		} else {
			s += 4
		}
	}
	if m.Method != "" {
		s += 2
	}
	s += len(m.Headers)
	s += len(m.Query)
	if len(m.BodyJSON) > 0 {
		s += 2
	}
	return s
}

func splitPath(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}

func isWildcard(segs []string) bool {
	if len(segs) == 0 {
		return false
	}
	last := segs[len(segs)-1]
	return last == "*"
}

func hasParam(segs []string) bool {
	for _, s := range segs {
		if isParam(s) {
			return true
		}
	}
	return false
}

func isParam(seg string) bool {
	return len(seg) >= 3 && seg[0] == '{' && seg[len(seg)-1] == '}'
}

// Request is a minimal representation of an HTTP request for matching.
type Request struct {
	Method   string
	Path     string
	Header   http.Header
	Query    url.Values
	BodyJSON map[string]interface{}
}

func equalJSON(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
