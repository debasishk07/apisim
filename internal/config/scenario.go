package config

import (
	"fmt"
	"strings"
)

// Version is the supported scenario file version.
const Version = 1

// Scenario is the top-level scenario file structure.
type Scenario struct {
	Version  int      `yaml:"version"`
	Name     string   `yaml:"name"`
	Listen   string   `yaml:"listen"`
	Seed     int64    `yaml:"seed"`
	Defaults Defaults `yaml:"defaults"`
	Routes   []Route  `yaml:"routes"`
	Webhooks Webhooks `yaml:"webhooks"`
}

// Defaults apply to all routes unless overridden.
type Defaults struct {
	Upstream      string `yaml:"upstream"`
	ForwardHeader bool   `yaml:"forward_headers"`
}

// Route matches incoming requests and applies behavior.
type Route struct {
	Name     string    `yaml:"name"`
	Match    Match     `yaml:"match"`
	Mode     string    `yaml:"mode"`
	Upstream string    `yaml:"upstream"`
	Response Response  `yaml:"response"`
	Failures []Failure `yaml:"failures"`
}

// Match describes how to match an incoming request.
type Match struct {
	Method   string                 `yaml:"method"`
	Path     string                 `yaml:"path"`
	Headers  map[string]string      `yaml:"headers"`
	Query    map[string]string      `yaml:"query"`
	BodyJSON map[string]interface{} `yaml:"body_json"`
}

// Response describes a mock response.
type Response struct {
	Status   int               `yaml:"status"`
	Headers  map[string]string `yaml:"headers"`
	Body     string            `yaml:"body"`
	BodyFile string            `yaml:"body_file"`
	Template bool              `yaml:"template"`
}

// Failure describes a failure injector applied to a route.
type Failure struct {
	Type       string                 `yaml:"type"`
	Trigger    map[string]interface{} `yaml:"when"`
	Latency    *Latency               `yaml:"-"`
	Error      *Error                 `yaml:"-"`
	Timeout    *Timeout               `yaml:"-"`
	DropConn   *DropConn              `yaml:"-"`
	MutateBody *MutateBody            `yaml:"-"`
	RateLimit  *RateLimit             `yaml:"-"`
	AuthExpiry *AuthExpiry            `yaml:"-"`
	SlowBody   *SlowBody              `yaml:"-"`
}

// Latency injector config.
type Latency struct {
	DelayMs  int `yaml:"delay_ms"`
	JitterMs int `yaml:"jitter_ms"`
}

// Error injector config.
type Error struct {
	Status  int               `yaml:"status"`
	Body    string            `yaml:"body"`
	Headers map[string]string `yaml:"headers"`
}

// Timeout injector config.
type Timeout struct{}

// DropConn injector config.
type DropConn struct{}

// MutateBody injector config.
type MutateBody struct {
	Operations []MutateOp `yaml:"operations"`
}

// MutateOp is a single body mutation operation.
type MutateOp struct {
	Op   string `yaml:"op"`
	Path string `yaml:"path"`
	To   string `yaml:"to"`
}

// RateLimit injector config.
type RateLimit struct {
	Limit   int               `yaml:"limit"`
	WindowS int               `yaml:"window_s"`
	Status  int               `yaml:"status"`
	Headers map[string]string `yaml:"headers"`
}

// AuthExpiry injector config.
type AuthExpiry struct {
	AfterRequests int `yaml:"after_requests"`
	Status        int `yaml:"status"`
}

// SlowBody injector config.
type SlowBody struct {
	BytesPerSecond int `yaml:"bytes_per_second"`
}

// ValidationError reports a config problem with a location.
type ValidationError struct {
	Path   string
	Line   int
	Column int
	Msg    string
}

func (e *ValidationError) Error() string {
	loc := ""
	if e.Line > 0 {
		loc = fmt.Sprintf(" (line %d", e.Line)
		if e.Column > 0 {
			loc += fmt.Sprintf(", col %d", e.Column)
		}
		loc += ")"
	}
	if e.Path != "" {
		return fmt.Sprintf("%s%s: %s", e.Path, loc, e.Msg)
	}
	return fmt.Sprintf("%s%s", loc, e.Msg)
}

// MultiError wraps multiple validation errors.
type MultiError []error

func (m MultiError) Error() string {
	b := &strings.Builder{}
	b.WriteString("invalid scenario:")
	for _, e := range m {
		fmt.Fprintf(b, "\n  - %v", e)
	}
	return b.String()
}

func (m MultiError) Unwrap() []error { return []error(m) }

// Known top-level and route fields, used for strict validation.
var topLevelFields = map[string]bool{
	"version": true, "name": true, "listen": true, "seed": true,
	"defaults": true, "routes": true, "webhooks": true,
}

var defaultsFields = map[string]bool{
	"upstream": true, "forward_headers": true,
}

var routeFields = map[string]bool{
	"name": true, "match": true, "mode": true, "upstream": true,
	"response": true, "failures": true,
}

var matchFields = map[string]bool{
	"method": true, "path": true, "headers": true, "query": true,
	"body_json": true,
}

var responseFields = map[string]bool{
	"status": true, "headers": true, "body": true, "body_file": true,
	"template": true,
}

var failureFields = map[string]bool{
	"type":        true,
	"when":        true,
	"delay_ms":    true,
	"jitter_ms":   true,
	"status":      true,
	"body":        true,
	"operations":  true,
	"limit":       true,
	"window_s":    true,
	"after_requests": true,
	"bytes_per_second": true,
	"error":       true,
	"headers":     true,
}

var latencyFields = map[string]bool{"delay_ms": true, "jitter_ms": true}
var errorFields = map[string]bool{"status": true, "body": true, "headers": true, "error": true}

// ValidModes are the supported route modes.
var ValidModes = map[string]bool{"proxy": true, "mock": true, "replay": true, "record": true}

// Webhooks configures webhook simulation.
type Webhooks struct {
	TargetURL    string   `yaml:"target_url"`
	PayloadTemplate string `yaml:"payload_template"`
	Events       []string `yaml:"events"`
	HMACSecret   string   `yaml:"hmac_secret"`
}

// ValidInjectorTypes are the supported failure injector types.
var ValidInjectorTypes = map[string]bool{
	"latency": true, "error": true, "timeout": true, "drop_connection": true,
	"mutate_body": true, "rate_limit": true, "auth_expiry": true, "slow_body": true,
}

// ValidTriggerKeys are the supported trigger conditions.
var ValidTriggerKeys = map[string]bool{
	"probability": true, "every_nth": true, "after_requests": true,
	"first_n_requests": true, "between_requests": true, "time_window": true,
}
