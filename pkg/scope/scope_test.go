package scope

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()

	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("failed to compile regexp %q: %v", pattern, err)
	}

	return re
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("failed to parse URL %q: %v", rawURL, err)
	}

	return u
}

func TestRuleMatchesURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rule     Rule
		url      *url.URL
		expected bool
	}{
		{
			name:     "url matches",
			rule:     Rule{URL: mustCompile(t, `^https://example\.com`)},
			url:      mustParseURL(t, "https://example.com/foo"),
			expected: true,
		},
		{
			name:     "url does not match",
			rule:     Rule{URL: mustCompile(t, `^https://example\.com`)},
			url:      mustParseURL(t, "https://other.com/foo"),
			expected: false,
		},
		{
			name:     "rule without url criterion",
			rule:     Rule{},
			url:      mustParseURL(t, "https://example.com/foo"),
			expected: false,
		},
		{
			name:     "nil request url",
			rule:     Rule{URL: mustCompile(t, `.*`)},
			url:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.rule.MatchesURL(tt.url); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestRuleMatchesHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rule     Rule
		header   http.Header
		expected bool
	}{
		{
			name: "key and value match the same header",
			rule: Rule{Header: Header{
				Key:   mustCompile(t, `^X-Token$`),
				Value: mustCompile(t, `^secret`),
			}},
			header:   http.Header{"X-Token": []string{"secret123"}},
			expected: true,
		},
		{
			name: "key and value match different headers",
			rule: Rule{Header: Header{
				Key:   mustCompile(t, `^X-Token$`),
				Value: mustCompile(t, `^secret`),
			}},
			header: http.Header{
				"X-Token": []string{"abc"},
				"X-Other": []string{"secret"},
			},
			expected: false,
		},
		{
			name: "only key set does not match",
			rule: Rule{Header: Header{
				Key: mustCompile(t, `^X-Token$`),
			}},
			header:   http.Header{"X-Token": []string{"secret123"}},
			expected: false,
		},
		{
			name: "only value set does not match",
			rule: Rule{Header: Header{
				Value: mustCompile(t, `^secret`),
			}},
			header:   http.Header{"X-Token": []string{"secret123"}},
			expected: false,
		},
		{
			name: "key matches but value does not",
			rule: Rule{Header: Header{
				Key:   mustCompile(t, `^X-Token$`),
				Value: mustCompile(t, `^secret`),
			}},
			header:   http.Header{"X-Token": []string{"abc"}},
			expected: false,
		},
		{
			name: "one of multiple values matches",
			rule: Rule{Header: Header{
				Key:   mustCompile(t, `^Accept$`),
				Value: mustCompile(t, `json`),
			}},
			header:   http.Header{"Accept": []string{"text/html", "application/json"}},
			expected: true,
		},
		{
			name:     "no header criterion",
			rule:     Rule{},
			header:   http.Header{"X-Token": []string{"secret123"}},
			expected: false,
		},
		{
			name: "no request headers",
			rule: Rule{Header: Header{
				Key:   mustCompile(t, `^X-Token$`),
				Value: mustCompile(t, `.*`),
			}},
			header:   http.Header{},
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.rule.MatchesHeader(tt.header); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestRuleMatchesBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rule     Rule
		body     []byte
		expected bool
	}{
		{
			name:     "literal regexp matches",
			rule:     Rule{Body: mustCompile(t, `secret`)},
			body:     []byte("this body contains a secret value"),
			expected: true,
		},
		{
			name:     "literal regexp does not match",
			rule:     Rule{Body: mustCompile(t, `secret`)},
			body:     []byte("nothing to see here"),
			expected: false,
		},
		{
			name:     "regexp with literal prefix matches",
			rule:     Rule{Body: mustCompile(t, `token=[0-9]+`)},
			body:     []byte("token=12345"),
			expected: true,
		},
		{
			name:     "regexp with literal prefix rejects body without prefix",
			rule:     Rule{Body: mustCompile(t, `token=[0-9]+`)},
			body:     []byte("12345"),
			expected: false,
		},
		{
			name:     "regexp without literal prefix matches",
			rule:     Rule{Body: mustCompile(t, `(?i)secret`)},
			body:     []byte("this is SECRET"),
			expected: true,
		},
		{
			name:     "empty body",
			rule:     Rule{Body: mustCompile(t, `.*`)},
			body:     nil,
			expected: false,
		},
		{
			name:     "no body criterion",
			rule:     Rule{},
			body:     []byte("secret"),
			expected: false,
		},
		{
			name:     "match at the end of a large body",
			rule:     Rule{Body: mustCompile(t, `needle`)},
			body:     append(bytes.Repeat([]byte("a"), 1<<20), []byte("needle")...),
			expected: true,
		},
		{
			name:     "large body without match",
			rule:     Rule{Body: mustCompile(t, `needle`)},
			body:     bytes.Repeat([]byte("a"), 1<<20),
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.rule.MatchesBody(tt.body); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestRuleMatch(t *testing.T) {
	t.Parallel()

	newReq := func(t *testing.T, rawURL string, headers map[string]string, body []byte) *http.Request {
		t.Helper()

		req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		for key, value := range headers {
			req.Header.Set(key, value)
		}

		return req
	}

	tests := []struct {
		name     string
		rule     Rule
		req      *http.Request
		body     []byte
		expected bool
	}{
		{
			name:     "url criterion matches",
			rule:     Rule{URL: mustCompile(t, `example\.com`)},
			req:      newReq(t, "http://example.com", nil, nil),
			expected: true,
		},
		{
			name: "header criterion matches",
			rule: Rule{Header: Header{
				Key:   mustCompile(t, `^X-Token$`),
				Value: mustCompile(t, `^secret`),
			}},
			req:      newReq(t, "http://other.com", map[string]string{"X-Token": "secret123"}, nil),
			expected: true,
		},
		{
			name:     "body criterion matches",
			rule:     Rule{Body: mustCompile(t, `needle`)},
			req:      newReq(t, "http://other.com", nil, []byte("find the needle")),
			body:     []byte("find the needle"),
			expected: true,
		},
		{
			name: "header key only does not match",
			rule: Rule{Header: Header{
				Key: mustCompile(t, `^X-Token$`),
			}},
			req:      newReq(t, "http://other.com", map[string]string{"X-Token": "secret123"}, nil),
			expected: false,
		},
		{
			name:     "empty rule does not match",
			rule:     Rule{},
			req:      newReq(t, "http://example.com", map[string]string{"X-Token": "secret"}, []byte("body")),
			body:     []byte("body"),
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.rule.Match(tt.req, tt.body); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestScopeMatch(t *testing.T) {
	t.Parallel()

	s := &Scope{}

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	if s.Match(req, nil) {
		t.Error("expected no match for scope without rules")
	}

	s.SetRules([]Rule{
		{URL: mustCompile(t, `^https://other\.com`)},
		{URL: mustCompile(t, `^http://example\.com`)},
	})

	if !s.Match(req, nil) {
		t.Error("expected match when one of the rules matches")
	}

	if got := len(s.Rules()); got != 2 {
		t.Errorf("expected 2 rules, got %v", got)
	}
}

func TestRuleMarshalUnmarshalBinary(t *testing.T) {
	t.Parallel()

	rule := Rule{
		URL: mustCompile(t, `^https://example\.com`),
		Header: Header{
			Key:   mustCompile(t, `^X-Token$`),
			Value: mustCompile(t, `^secret`),
		},
		Body: mustCompile(t, `needle`),
	}

	data, err := rule.MarshalBinary()
	if err != nil {
		t.Fatalf("failed to marshal rule: %v", err)
	}

	var got Rule
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("failed to unmarshal rule: %v", err)
	}

	if got.URL.String() != rule.URL.String() {
		t.Errorf("expected URL pattern %q, got %q", rule.URL.String(), got.URL.String())
	}

	if got.Header.Key.String() != rule.Header.Key.String() {
		t.Errorf("expected header key pattern %q, got %q", rule.Header.Key.String(), got.Header.Key.String())
	}

	if got.Header.Value.String() != rule.Header.Value.String() {
		t.Errorf("expected header value pattern %q, got %q", rule.Header.Value.String(), got.Header.Value.String())
	}

	if got.Body.String() != rule.Body.String() {
		t.Errorf("expected body pattern %q, got %q", rule.Body.String(), got.Body.String())
	}
}

func TestRuleMarshalUnmarshalBinaryEmpty(t *testing.T) {
	t.Parallel()

	data, err := Rule{}.MarshalBinary()
	if err != nil {
		t.Fatalf("failed to marshal empty rule: %v", err)
	}

	var got Rule
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("failed to unmarshal empty rule: %v", err)
	}

	if got.URL != nil || got.Header.Key != nil || got.Header.Value != nil || got.Body != nil {
		t.Error("expected all criteria to be nil")
	}
}

func TestRuleUnmarshalBinaryInvalidPattern(t *testing.T) {
	t.Parallel()

	// Encode a DTO containing invalid regexp patterns directly, simulating
	// persisted data that can no longer be compiled.
	dto := ruleDTO{
		URL:  `^https://example\.com`,
		Body: `[invalid`,
	}
	dto.Header.Key = `(unclosed`
	dto.Header.Value = `^secret`

	buf := bytes.Buffer{}
	if err := gob.NewEncoder(&buf).Encode(dto); err != nil {
		t.Fatalf("failed to encode DTO: %v", err)
	}

	var got Rule
	if err := got.UnmarshalBinary(buf.Bytes()); err != nil {
		t.Fatalf("expected invalid patterns to be dropped, got error: %v", err)
	}

	if got.URL == nil || got.URL.String() != dto.URL {
		t.Errorf("expected valid URL pattern %q to be loaded, got %v", dto.URL, got.URL)
	}

	if got.Header.Key != nil {
		t.Errorf("expected invalid header key pattern to be dropped, got %v", got.Header.Key)
	}

	if got.Header.Value == nil || got.Header.Value.String() != dto.Header.Value {
		t.Errorf("expected valid header value pattern %q to be loaded, got %v", dto.Header.Value, got.Header.Value)
	}

	if got.Body != nil {
		t.Errorf("expected invalid body pattern to be dropped, got %v", got.Body)
	}
}

func TestRuleUnmarshalBinaryCorruptData(t *testing.T) {
	t.Parallel()

	var got Rule
	if err := got.UnmarshalBinary([]byte("not a gob encoded rule")); err == nil {
		t.Error("expected error for corrupt data, got nil")
	}
}

func BenchmarkRuleMatchesBody(b *testing.B) {
	rule := Rule{Body: regexp.MustCompile(`needle=[0-9]+`)}
	body := []byte(strings.Repeat("a", 1<<20))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rule.MatchesBody(body)
	}
}
