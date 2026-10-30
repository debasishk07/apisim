package mock

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
	"apisim/internal/clock"
)

type MockResponse struct {
	Status   int
	Headers  map[string]string
	Body     string
	BodyFile string
	Template bool
}

func (m *MockResponse) MockedResponse(clock clock.Clock, c *Context) (int, http.Header, io.ReadCloser, error) {
	status := m.Status
	headers := http.Header{}

	if m.Template && m.Body != "" {
		tmpl, err := template.New("mock").Parse(m.Body)
		if err != nil {
			return 0, nil, nil, err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, c); err != nil {
			return 0, nil, nil, err
		}
		bodyBytes := buf.Bytes()
		n, err := strconv.Atoi(parseStatusFromTemplate(tmpl, m.Body))
		if err != nil {
			return 0, nil, nil, err
		}
		status = n
		headers.Set("Content-Type", "application/json")
		return status, headers, io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}

	if m.BodyFile != "" {
		data, err := os.ReadFile(m.BodyFile)
		if err != nil {
			return 0, nil, nil, err
		}
		if m.Template {
			tmpl, err := template.New("mock_file").Parse(string(data))
			if err != nil {
				return 0, nil, nil, err
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, c); err != nil {
				return 0, nil, nil, err
			}
			bodyBytes := buf.Bytes()
			n, err := strconv.Atoi(parseStatusFromTemplate(tmpl, string(data)))
			if err != nil {
				return 0, nil, nil, err
			}
			status = n
			headers.Set("Content-Type", "application/json")
			return status, headers, io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		headers.Set("Content-Type", "application/json")
		return status, headers, io.NopCloser(bytes.NewReader(data)), nil
	}

	headers.Set("Content-Type", "application/json")
	return status, headers, io.NopCloser(strings.NewReader(m.Body)), nil
}

func parseStatusFromTemplate(t *template.Template, body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{{ status ") {
			str := strings.TrimPrefix(trimmed, "{{ status ")
			str = strings.TrimSuffix(str, " }}"  )
			return str
		}
	}
	return "0"
}

type Context struct {
	req *http.Request
}

func newContext(req *http.Request) *Context {
	return &Context{req: req}
}

func (c *Context) Path(key string) string {
	return c.req.URL.Path
}

func (c *Context) Query(key string) string {
	return c.req.URL.Query().Get(key)
}

func (c *Context) Header(key string) string {
	return c.req.Header.Get(key)
}

func uuid() string {
	return fmt.Sprintf("uuid-%d", time.Now().UnixNano())
}

func randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	return rand.Intn(max)
}
var _ = uuid()
var _ = randomInt(0)
var _ = newContext(nil)
