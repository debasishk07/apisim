package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFromFile reads and strictly validates a scenario file.
func LoadFromFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read scenario file: %w", err)
	}
	return Load(data)
}

// Load parses and strictly validates scenario YAML bytes.
func Load(data []byte) (*Scenario, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, &ValidationError{Msg: "expected a single YAML document"}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, &ValidationError{Path: "<root>", Line: root.Line, Column: root.Column, Msg: "expected a mapping"}
	}

	scen := &Scenario{}
	var errs MultiError

	// Walk root mapping pairs to check unknown fields and collect known ones.
	for i := 0; i < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]
		key := keyNode.Value

		if !topLevelFields[key] {
			errs = append(errs, &ValidationError{
				Path:   "<root>",
				Line:   keyNode.Line,
				Column: keyNode.Column,
				Msg:    fmt.Sprintf("unknown field %q", key),
			})
			continue
		}

		switch key {
		case "webhooks":
		// Basic webhooks config validation - store raw values
		scen.Webhooks = Webhooks{}
		if valNode.Kind == yaml.MappingNode {
			for i := 0; i < len(valNode.Content); i += 2 {
				k := valNode.Content[i].Value
				v := valNode.Content[i+1]
				switch k {
				case "target_url":
					scen.Webhooks.TargetURL = unquote(v.Value)
				case "payload_template":
					scen.Webhooks.PayloadTemplate = unquote(v.Value)
				case "events":
					// events is a sequence, basic validation
					if v.Kind == yaml.SequenceNode {
						for _, item := range v.Content {
							if item.Kind == yaml.ScalarNode {
								scen.Webhooks.Events = append(scen.Webhooks.Events, unquote(item.Value))
							}
						}
					}
				case "hmac_secret":
					scen.Webhooks.HMACSecret = unquote(v.Value)
				}
			}
		}
		continue
	case "version":
			v, err := parseInt(valNode, keyNode)
			if err != nil {
				errs = append(errs, err)
			} else {
				scen.Version = v
			}
		case "name":
			scen.Name = unquote(valNode.Value)
		case "listen":
			scen.Listen = unquote(valNode.Value)
		case "seed":
			s, err := parseInt(valNode, keyNode)
			if err != nil {
				errs = append(errs, err)
			} else {
				scen.Seed = int64(s)
			}
		case "defaults":
			d, err := parseDefaults(valNode, keyNode)
			if err != nil {
				errs = append(errs, err)
			} else {
				scen.Defaults = d
			}
		case "routes":
			routes, err := parseRoutes(valNode, keyNode)
			if err != nil {
				errs = append(errs, err)
			} else {
				scen.Routes = routes
			}
		}
	}

	// Value-level validation.
	if scen.Version != Version {
		errs = append(errs, &ValidationError{
			Path: "version",
			Msg:  fmt.Sprintf("unsupported version %d; only version %d is supported", scen.Version, Version),
		})
	}

	if len(scen.Routes) == 0 {
		errs = append(errs, &ValidationError{
			Path: "routes",
			Msg:  "at least one route is required",
		})
	}

	if len(errs) > 0 {
		return nil, errs
	}

	return scen, nil
}

func parseDefaults(node *yaml.Node, keyNode *yaml.Node) (Defaults, error) {
	var d Defaults
	if node.Kind != yaml.MappingNode {
		return d, &ValidationError{Path: "defaults", Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a mapping"}
	}

	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		if !defaultsFields[k] {
			return d, &ValidationError{Path: "defaults", Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q", k)}
		}
		switch k {
		case "upstream":
			d.Upstream = unquote(v.Value)
		case "forward_headers":
			b, err := parseBool(v, node.Content[i])
			if err != nil {
				return d, err
			}
			d.ForwardHeader = b
		}
	}
	return d, nil
}

func parseRoutes(node *yaml.Node, keyNode *yaml.Node) ([]Route, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, MultiError{&ValidationError{Path: "routes", Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a sequence"}}
	}

	var routes []Route
	var errs MultiError
	for idx, item := range node.Content {
		r, errs2 := parseRoute(item, idx)
		errs = append(errs, errs2...)
		routes = append(routes, r)
	}
	if len(errs) > 0 {
		return routes, errs
	}
	return routes, nil
}

func parseRoute(node *yaml.Node, idx int) (Route, MultiError) {
	var errs MultiError
	var r Route

	if node.Kind != yaml.MappingNode {
		errs = append(errs, &ValidationError{Path: fmt.Sprintf("routes[%d]", idx), Line: node.Line, Column: node.Column, Msg: "expected a mapping"})
		return r, errs
	}

	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		path := fmt.Sprintf("routes[%d]", idx)
		if !routeFields[k] {
			errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q", k)})
			continue
		}

		switch k {
		case "name":
			r.Name = unquote(v.Value)
		case "match":
			m, err := parseMatch(v, node.Content[i], idx)
			if err != nil {
				errs = append(errs, err)
			} else {
				r.Match = m
			}
		case "mode":
			r.Mode = unquote(v.Value)
			if !ValidModes[r.Mode] {
				errs = append(errs, &ValidationError{Path: path + ".mode", Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown mode %q; valid modes: proxy, mock, replay", r.Mode)})
			}
		case "upstream":
			r.Upstream = unquote(v.Value)
		case "response":
			resp, err := parseResponse(v, node.Content[i], idx)
			if err != nil {
				errs = append(errs, err)
			} else {
				r.Response = resp
			}
		case "failures":
			fails, err := parseFailures(v, node.Content[i], idx)
			if err != nil {
				errs = append(errs, err)
			} else {
				r.Failures = fails
			}
		}
	}

	if r.Name == "" {
		errs = append(errs, &ValidationError{Path: fmt.Sprintf("routes[%d]", idx) + ".name", Line: node.Line, Column: node.Column, Msg: "route name is required"})
	}
	if r.Mode == "" {
		r.Mode = "proxy"
	}
	return r, errs
}

func parseMatch(node *yaml.Node, keyNode *yaml.Node, idx int) (Match, MultiError) {
	var errs MultiError
	var m Match
	path := fmt.Sprintf("routes[%d].match", idx)

	if node.Kind != yaml.MappingNode {
		errs = append(errs, &ValidationError{Path: path, Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a mapping"})
		return m, errs
	}

	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		if !matchFields[k] {
			errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q", k)})
			continue
		}
		switch k {
		case "method":
			m.Method = strings.ToUpper(unquote(v.Value))
		case "path":
			m.Path = unquote(v.Value)
		case "headers":
			mp, err := parseStringMap(v, node.Content[i], path+".headers")
			if err != nil {
				errs = append(errs, err)
			} else {
				m.Headers = mp
			}
		case "query":
			mp, err := parseStringMap(v, node.Content[i], path+".query")
			if err != nil {
				errs = append(errs, err)
			} else {
				m.Query = mp
			}
		case "body_json":
			obj, err := parseBodyJSON(v, node.Content[i], path+".body_json")
			if err != nil {
				errs = append(errs, err)
			} else {
				m.BodyJSON = obj
			}
		}
	}
	return m, errs
}

func parseResponse(node *yaml.Node, keyNode *yaml.Node, idx int) (Response, MultiError) {
	var errs MultiError
	var r Response
	path := fmt.Sprintf("routes[%d].response", idx)

	if node.Kind != yaml.MappingNode {
		errs = append(errs, &ValidationError{Path: path, Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a mapping"})
		return r, errs
	}

	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		if !responseFields[k] {
			errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q", k)})
			continue
		}
		switch k {
		case "status":
			s, err := parseInt(v, node.Content[i])
			if err != nil {
				errs = append(errs, err)
			} else {
				r.Status = s
			}
		case "headers":
			mp, err := parseStringMap(v, node.Content[i], path+".headers")
			if err != nil {
				errs = append(errs, err)
			} else {
				r.Headers = mp
			}
		case "body":
			r.Body = unquote(v.Value)
		case "body_file":
			r.BodyFile = unquote(v.Value)
		case "template":
			b, err := parseBool(v, node.Content[i])
			if err != nil {
				errs = append(errs, err)
			} else {
				r.Template = b
			}
		}
	}
	return r, errs
}

func parseFailures(node *yaml.Node, keyNode *yaml.Node, idx int) ([]Failure, MultiError) {
	var errs MultiError
	if node.Kind != yaml.SequenceNode {
		return nil, MultiError{&ValidationError{Path: fmt.Sprintf("routes[%d].failures", idx), Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a sequence"}}
	}

	var fails []Failure
	for fidx, item := range node.Content {
		f, errs2 := parseFailure(item, idx, fidx)
		errs = append(errs, errs2...)
		fails = append(fails, f)
	}
	return fails, errs
}

func parseFailure(node *yaml.Node, ridx, fidx int) (Failure, MultiError) {
	var errs MultiError
	var f Failure
	path := fmt.Sprintf("routes[%d].failures[%d]", ridx, fidx)

	if node.Kind != yaml.MappingNode {
		errs = append(errs, &ValidationError{Path: path, Line: node.Line, Column: node.Column, Msg: "expected a mapping"})
		return f, errs
	}

	typeField := ""
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		if !failureFields[k] {
			errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q", k)})
			continue
		}
		switch k {
		case "type":
			typeField = unquote(v.Value)
			f.Type = typeField
			if !ValidInjectorTypes[typeField] {
				suggest := didYouMean(typeField, validInjectorKeys())
				errs = append(errs, &ValidationError{Path: path + ".type", Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown failure type %q%s", typeField, suggest)})
			}
		case "when":
			trig, err := parseTrigger(v, node.Content[i], path)
			if err != nil {
				errs = append(errs, err)
			} else {
				f.Trigger = trig
			}
		case "delay_ms", "jitter_ms":
			// handled below per type
		case "status", "body":
			// handled below per type
		case "operations":
			// handled below per type
		case "limit", "window_s":
			// handled below per type
		case "after_requests":
			// handled below per type
		case "bytes_per_second":
			// handled below per type
		}
	}

	// Type-specific validation.
	switch typeField {
	case "latency":
		lat := &Latency{}
		allowed := latencyFields
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1]
			if !allowed[k] {
				errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q for latency", k)})
				continue
			}
			switch k {
			case "delay_ms":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					lat.DelayMs = n
				}
			case "jitter_ms":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					lat.JitterMs = n
				}
			}
		}
		f.Latency = lat
	case "error":
		errCfg := &Error{}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1]
			// Skip type and when fields (already handled above).
			if k == "type" || k == "when" {
				continue
			}
			if !errorFields[k] {
				errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q for error", k)})
				continue
			}
			switch k {
		case "error":
				errCfg.Body = unquote(v.Value)
			case "status":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					errCfg.Status = n
				}
			case "body":
				errCfg.Body = unquote(v.Value)
			case "headers":
				mp, err := parseStringMap(v, node.Content[i], path+".headers")
				if err != nil {
					errs = append(errs, err)
				} else {
					errCfg.Headers = mp
				}
			}
		}
		f.Error = errCfg
	case "timeout":
		f.Timeout = &Timeout{}
	case "drop_connection":
		f.DropConn = &DropConn{}
	case "mutate_body":
		mb := &MutateBody{}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1]
			if k != "operations" {
				errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q for mutate_body", k)})
				continue
			}
			ops, err := parseMutateOps(v, node.Content[i], path)
			if err != nil {
				errs = append(errs, err)
			} else {
				mb.Operations = ops
			}
		}
		f.MutateBody = mb
	case "rate_limit":
		rl := &RateLimit{}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1]
			switch k {
			case "limit":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					rl.Limit = n
				}
			case "window_s":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					rl.WindowS = n
				}
			case "status":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					rl.Status = n
				}
			case "headers":
				mp, err := parseStringMap(v, node.Content[i], path+".headers")
				if err != nil {
					errs = append(errs, err)
				} else {
					rl.Headers = mp
				}
			default:
				errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q for rate_limit", k)})
			}
		}
		f.RateLimit = rl
	case "auth_expiry":
		ae := &AuthExpiry{}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1]
			switch k {
			case "after_requests":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					ae.AfterRequests = n
				}
			case "status":
				n, err := parseInt(v, node.Content[i])
				if err != nil {
					errs = append(errs, err)
				} else {
					ae.Status = n
				}
			default:
				errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q for auth_expiry", k)})
			}
		}
		f.AuthExpiry = ae
	case "slow_body":
		sb := &SlowBody{}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1]
			if k != "bytes_per_second" {
				errs = append(errs, &ValidationError{Path: path, Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown field %q for slow_body", k)})
				continue
			}
			n, err := parseInt(v, node.Content[i])
			if err != nil {
				errs = append(errs, err)
			} else {
				sb.BytesPerSecond = n
			}
		}
		f.SlowBody = sb
	}
	return f, errs
}

func parseTrigger(node *yaml.Node, keyNode *yaml.Node, path string) (map[string]interface{}, error) {
	if node.Kind != yaml.MappingNode {
		return nil, &ValidationError{Path: path + ".when", Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a mapping"}
	}
	trig := map[string]interface{}{}
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		if !ValidTriggerKeys[k] {
			return nil, &ValidationError{Path: path + ".when", Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: fmt.Sprintf("unknown trigger %q; valid: probability, every_nth, after_requests, first_n_requests, between_requests, time_window", k)}
		}
		switch k {
		case "probability":
			p, err := parseFloat(v, node.Content[i])
			if err != nil {
				return nil, err
			}
			if p < 0 || p > 1 {
				return nil, &ValidationError{Path: path + ".when.probability", Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: "probability must be between 0 and 1"}
			}
			trig[k] = p
		case "every_nth", "after_requests", "first_n_requests":
			n, err := parseInt(v, node.Content[i])
			if err != nil {
				return nil, err
			}
			trig[k] = n
		case "between_requests":
			arr, err := parseIntArray(v, node.Content[i])
			if err != nil {
				return nil, err
			}
			if len(arr) != 2 {
				return nil, &ValidationError{Path: path + ".when.between_requests", Line: node.Content[i].Line, Column: node.Content[i].Column, Msg: "between_requests must be [a, b]"}
			}
			trig[k] = arr
		case "time_window":
			trig[k] = unquote(v.Value)
		}
	}
	return trig, nil
}

func parseMutateOps(node *yaml.Node, keyNode *yaml.Node, path string) ([]MutateOp, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, &ValidationError{Path: path + ".operations", Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a sequence"}
	}
	var ops []MutateOp
	for oidx, item := range node.Content {
		op := MutateOp{}
		if item.Kind != yaml.MappingNode {
			return nil, &ValidationError{Path: fmt.Sprintf("%s[%d]", path+".operations", oidx), Line: item.Line, Column: item.Column, Msg: "expected a mapping"}
		}
		for i := 0; i < len(item.Content); i += 2 {
			k := item.Content[i].Value
			v := item.Content[i+1]
			switch k {
			case "op":
				op.Op = unquote(v.Value)
			case "path":
				op.Path = unquote(v.Value)
			case "to":
				op.To = unquote(v.Value)
			default:
				return nil, &ValidationError{Path: fmt.Sprintf("%s[%d]", path+".operations", oidx), Line: item.Content[i].Line, Column: item.Content[i].Column, Msg: fmt.Sprintf("unknown field %q", k)}
			}
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func parseStringMap(node *yaml.Node, keyNode *yaml.Node, path string) (map[string]string, error) {
	if node.Kind != yaml.MappingNode {
		return nil, &ValidationError{Path: path, Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a mapping"}
	}
	m := map[string]string{}
	for i := 0; i < len(node.Content); i += 2 {
		k := unquote(node.Content[i].Value)
		v := unquote(node.Content[i+1].Value)
		m[k] = v
	}
	return m, nil
}

func parseBodyJSON(node *yaml.Node, keyNode *yaml.Node, path string) (map[string]interface{}, error) {
	if node.Kind != yaml.MappingNode {
		return nil, &ValidationError{Path: path, Line: keyNode.Line, Column: keyNode.Column, Msg: "expected a mapping"}
	}
	m := map[string]interface{}{}
	for i := 0; i < len(node.Content); i += 2 {
		k := unquote(node.Content[i].Value)
		m[k] = yamlValue(node.Content[i+1])
	}
	return m, nil
}

func yamlValue(node *yaml.Node) interface{} {
	switch node.Kind {
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!int":
			n, _ := strconv.Atoi(node.Value)
			return n
		case "!!float":
			n, _ := strconv.ParseFloat(node.Value, 64)
			return n
		case "!!bool":
			b, _ := strconv.ParseBool(node.Value)
			return b
		default:
			return unquote(node.Value)
		}
	case yaml.MappingNode:
		m := map[string]interface{}{}
		for i := 0; i < len(node.Content); i += 2 {
			m[unquote(node.Content[i].Value)] = yamlValue(node.Content[i+1])
		}
		return m
	case yaml.SequenceNode:
		arr := []interface{}{}
		for _, item := range node.Content {
			arr = append(arr, yamlValue(item))
		}
		return arr
	default:
		return unquote(node.Value)
	}
}

func parseInt(node *yaml.Node, keyNode *yaml.Node) (int, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		n, err := strconv.Atoi(node.Value)
		if err != nil {
			return 0, &ValidationError{Path: keyNode.Value, Line: node.Line, Column: node.Column, Msg: fmt.Sprintf("expected integer, got %q", node.Value)}
		}
		return n, nil
	default:
		return 0, &ValidationError{Path: keyNode.Value, Line: node.Line, Column: node.Column, Msg: "expected integer"}
	}
}

func parseFloat(node *yaml.Node, keyNode *yaml.Node) (float64, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		n, err := strconv.ParseFloat(node.Value, 64)
		if err != nil {
			return 0, &ValidationError{Path: keyNode.Value, Line: node.Line, Column: node.Column, Msg: fmt.Sprintf("expected number, got %q", node.Value)}
		}
		return n, nil
	default:
		return 0, &ValidationError{Path: keyNode.Value, Line: node.Line, Column: node.Column, Msg: "expected number"}
	}
}

func parseBool(node *yaml.Node, keyNode *yaml.Node) (bool, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return strconv.ParseBool(node.Value)
	default:
		return false, &ValidationError{Path: keyNode.Value, Line: node.Line, Column: node.Column, Msg: "expected boolean"}
	}
}

func parseIntArray(node *yaml.Node, keyNode *yaml.Node) ([]int, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, &ValidationError{Path: keyNode.Value, Line: node.Line, Column: node.Column, Msg: "expected a sequence"}
	}
	arr := []int{}
	for _, item := range node.Content {
		n, err := parseInt(item, item)
		if err != nil {
			return nil, err
		}
		arr = append(arr, n)
	}
	return arr, nil
}

func validInjectorKeys() []string {
	keys := []string{}
	for k := range ValidInjectorTypes {
		keys = append(keys, k)
	}
	return keys
}

func didYouMean(input string, candidates []string) string {
	best := ""
	bestDist := 999
	for _, c := range candidates {
		d := levenshtein(input, c)
		if d < bestDist && d <= 2 {
			bestDist = d
			best = c
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf(", did you mean %q?", best)
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			dp[i][j] = min(min(dp[i-1][j]+1, dp[i][j-1]+1), dp[i-1][j-1]+cost)
		}
	}
	return dp[la][lb]
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
