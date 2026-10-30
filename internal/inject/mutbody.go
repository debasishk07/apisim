package inject

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MutateBody applies JSON mutation operations to a response body.
type MutateBody struct {
	operations []MutateOp
	trigger    Trigger
}

type MutateOp struct {
	Op      string
	Path    string   // JSONPath like $.field.sub
	To      string   // target for rename / type
	AtBytes int      // for truncate
}

func NewMutateBody(ops []MutateOp, when map[string]interface{}, rng *rand.Rand) (*MutateBody, error) {
	trigger, err := ParseTrigger(when, rng)
	if err != nil {
		return nil, err
	}
	return &MutateBody{operations: ops, trigger: trigger}, nil
}

func (m *MutateBody) Name() string { return "mutate_body" }

func (m *MutateBody) BeforeRequest(w http.ResponseWriter, req *http.Request, c Counters, now time.Time) (*http.Response, error) {
	return nil, nil
}

func (m *MutateBody) AfterResponse(resp *http.Response, c Counters, now time.Time) (*http.Response, error) {
	if !m.trigger.Fire(c, now) || resp.Body == nil {
		return resp, nil
	}
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return resp, nil
	}

	for _, op := range m.operations {
		switch op.Op {
		case "truncate":
			if op.AtBytes > 0 && op.AtBytes < len(raw) {
				raw = raw[:op.AtBytes]
			}
		case "remove_field", "rename_field", "change_type":
			var doc interface{}
			if err := json.Unmarshal(raw, &doc); err != nil {
				continue
			}
			var changed bool
			switch op.Op {
			case "remove_field":
				changed = jsonRemove(doc, op.Path)
			case "rename_field":
				changed = jsonRename(doc, op.Path, op.To)
			case "change_type":
				changed = jsonChangeType(doc, op.Path, op.To)
			}
			if !changed {
				continue
			}
			b, err := json.Marshal(doc)
			if err != nil {
				continue
			}
			raw = b
		}
	}

	resp.Body = io.NopCloser(bytes.NewReader(raw))
	resp.ContentLength = int64(len(raw))
	resp.Header.Del("Content-Length")
	return resp, nil
}

// jsonPath parses "$.a.b[0].c" into segments.
func jsonPath(path string) []string {
	if !strings.HasPrefix(path, "$.") {
		return nil
	}
	rest := path[2:]
	var segs []string
	cur := strings.Builder{}
	inBracket := false
	for _, ch := range rest {
		switch ch {
		case '.':
			if !inBracket {
				if cur.Len() > 0 {
					segs = append(segs, cur.String())
					cur.Reset()
				}
				continue
			}
			cur.WriteRune(ch)
		case '[':
			inBracket = true
			cur.WriteRune(ch)
		case ']':
			inBracket = false
			cur.WriteRune(ch)
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		segs = append(segs, cur.String())
	}
	return segs
}

// navigateToParent returns the container holding the value at segs[-1] and
// the key/index used to access it. For a single-segment path the container
// is the document itself.
func navigateToParent(doc interface{}, segs []string) (interface{}, interface{}, bool) {
	if len(segs) == 0 {
		return nil, nil, false
	}
	key := interface{}(segs[len(segs)-1])
	if len(segs) == 1 {
		switch doc.(type) {
		case map[string]interface{}:
			if k, ok := key.(string); ok {
				return doc, k, true
			}
		case []interface{}:
			if k, ok := key.(string); ok {
				if _, err := strconv.Atoi(strings.Trim(k, "[]")); err == nil {
					return doc, k, true
				}
			}
		}
		return nil, nil, false
	}
	var cur interface{} = doc
	for _, seg := range segs[:len(segs)-1] {
		switch c := cur.(type) {
		case map[string]interface{}:
			val, ok := c[seg]
			if !ok {
				return nil, nil, false
			}
			cur = val
		case []interface{}:
			idx, err := strconv.Atoi(strings.Trim(seg, "[]"))
			if err != nil || idx < 0 || idx >= len(c) {
				return nil, nil, false
			}
			cur = c[idx]
		default:
			return nil, nil, false
		}
	}
	switch p := cur.(type) {
	case map[string]interface{}:
		if k, ok := key.(string); ok {
			if _, ex := p[k]; ex {
				return p, key, true
			}
		}
	case []interface{}:
		if k, ok := key.(string); ok {
			if idx, err := strconv.Atoi(strings.Trim(k, "[]")); err == nil && idx >= 0 && idx < len(p) {
				return p, key, true
			}
		}
	}
	return nil, nil, false
}

func getValue(parent interface{}, key interface{}) (interface{}, bool) {
	switch p := parent.(type) {
	case map[string]interface{}:
		if k, ok := key.(string); ok {
			v, ex := p[k]
			return v, ex
		}
	case []interface{}:
		if k, ok := key.(string); ok {
			if idx, err := strconv.Atoi(strings.Trim(k, "[]")); err == nil && idx >= 0 && idx < len(p) {
				return p[idx], true
			}
		}
	}
	return nil, false
}

func setValue(parent interface{}, key interface{}, val interface{}) bool {
	switch p := parent.(type) {
	case map[string]interface{}:
		if k, ok := key.(string); ok {
			p[k] = val
			return true
		}
	case []interface{}:
		if k, ok := key.(string); ok {
			if idx, err := strconv.Atoi(strings.Trim(k, "[]")); err == nil && idx >= 0 && idx < len(p) {
				p[idx] = val
				return true
			}
		}
	}
	return false
}

func jsonRemove(doc interface{}, path string) bool {
	segs := jsonPath(path)
	if len(segs) == 0 {
		return false
	}
	parent, key, ok := navigateToParent(doc, segs)
	if !ok {
		return false
	}
	switch p := parent.(type) {
	case map[string]interface{}:
		delete(p, key.(string))
		return true
	case []interface{}:
		if k, ok := key.(string); ok {
			if idx, err := strconv.Atoi(strings.Trim(k, "[]")); err == nil && idx >= 0 && idx < len(p) {
				p[idx] = nil
				return true
			}
		}
	}
	return false
}

func jsonRename(doc interface{}, path, to string) bool {
	segs := jsonPath(path)
	tosegs := jsonPath(to)
	if len(segs) == 0 || len(tosegs) == 0 {
		return false
	}
	parent, key, ok := navigateToParent(doc, segs)
	if !ok {
		return false
	}
	val, _ := getValue(parent, key)
	switch p := parent.(type) {
	case map[string]interface{}:
		delete(p, key.(string))
	case []interface{}:
		if k, ok := key.(string); ok {
			if idx, err := strconv.Atoi(strings.Trim(k, "[]")); err == nil && idx >= 0 && idx < len(p) {
				p[idx] = nil
			}
		}
	}
	tparent, tkey, ok := navigateToParent(doc, tosegs)
	if !ok {
		return false
	}
	return setValue(tparent, tkey, val)
}

func jsonChangeType(doc interface{}, path, to string) bool {
	segs := jsonPath(path)
	if len(segs) == 0 {
		return false
	}
	parent, key, ok := navigateToParent(doc, segs)
	if !ok {
		return false
	}
	val, existed := getValue(parent, key)
	if !existed {
		return false
	}
	var converted interface{}
	switch to {
	case "string":
		converted = fmt.Sprintf("%v", val)
	case "number":
		if f, ok := val.(float64); ok {
			converted = f
		} else if n, err := strconv.Atoi(fmt.Sprintf("%v", val)); err == nil {
			converted = float64(n)
		} else {
			converted = val
		}
	case "boolean":
		converted = fmt.Sprintf("%v", val) == "true"
	default:
		converted = val
	}
	if !setValue(parent, key, converted) {
		return false
	}
	return true
}
