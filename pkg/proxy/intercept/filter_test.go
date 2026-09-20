package intercept_test

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/dstotijn/hetty/pkg/proxy/intercept"
	"github.com/dstotijn/hetty/pkg/scope"
)

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()

	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("failed to compile regexp %q: %v", pattern, err)
	}

	return re
}

func scopeWithRules(rules ...scope.Rule) *scope.Scope {
	s := &scope.Scope{}
	s.SetRules(rules)

	return s
}

func TestMatchRequestScope(t *testing.T) {
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
		rules    []scope.Rule
		req      *http.Request
		expected bool
	}{
		{
			name: "url matches",
			rules: []scope.Rule{
				{URL: mustCompile(t, `^http://example\.com`)},
			},
			req:      newReq(t, "http://example.com/foo", nil, nil),
			expected: true,
		},
		{
			name: "header key and value match",
			rules: []scope.Rule{
				{Header: scope.Header{
					Key:   mustCompile(t, `^X-Token$`),
					Value: mustCompile(t, `^secret`),
				}},
			},
			req:      newReq(t, "http://example.com", map[string]string{"X-Token": "secret123"}, nil),
			expected: true,
		},
		{
			name: "header key only does not match",
			rules: []scope.Rule{
				{Header: scope.Header{Key: mustCompile(t, `^X-Token$`)}},
			},
			req:      newReq(t, "http://example.com", map[string]string{"X-Token": "secret123"}, nil),
			expected: false,
		},
		{
			name: "body matches",
			rules: []scope.Rule{
				{Body: mustCompile(t, `needle`)},
			},
			req:      newReq(t, "http://example.com", nil, []byte("find the needle")),
			expected: true,
		},
		{
			name: "body does not match",
			rules: []scope.Rule{
				{Body: mustCompile(t, `needle`)},
			},
			req:      newReq(t, "http://example.com", nil, []byte("nothing here")),
			expected: false,
		},
		{
			name:     "no rules",
			rules:    nil,
			req:      newReq(t, "http://example.com", nil, nil),
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			match, err := intercept.MatchRequestScope(tt.req, scopeWithRules(tt.rules...))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if match != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, match)
			}
		})
	}
}

func TestMatchRequestScopeBodyIsRestored(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPost, "http://example.com", bytes.NewReader([]byte("find the needle")))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	s := scopeWithRules(scope.Rule{Body: mustCompile(t, `needle`)})

	match, err := intercept.MatchRequestScope(req, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !match {
		t.Error("expected request to be in scope")
	}

	// The request body must still be readable after scope matching.
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read request body after match: %v", err)
	}

	if string(body) != "find the needle" {
		t.Errorf("expected request body to be restored, got %q", string(body))
	}
}

func TestMatchRequestScopeNilBody(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Body = nil

	s := scopeWithRules(scope.Rule{Body: mustCompile(t, `needle`)})

	match, err := intercept.MatchRequestScope(req, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if match {
		t.Error("expected no match for request without body")
	}
}
