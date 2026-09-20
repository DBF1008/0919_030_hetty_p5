package scope

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"regexp"
	"testing"
)

func mustRegexp(t *testing.T, expr string) *regexp.Regexp {
	t.Helper()

	re, err := regexp.Compile(expr)
	if err != nil {
		t.Fatalf("failed to compile regexp %q: %v", expr, err)
	}

	return re
}

func newRequest(t *testing.T, url string, header http.Header) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header = header

	return req
}

func TestRuleMatchURL(t *testing.T) {
	t.Parallel()

	rule := Rule{URL: mustRegexp(t, `^https://example\.com`)}
	req := newRequest(t, "https://example.com/foo", nil)

	if !rule.Match(req, nil) {
		t.Error("expected rule to match request URL")
	}

	req = newRequest(t, "https://other.org/foo", nil)

	if rule.Match(req, nil) {
		t.Error("expected rule to not match request URL")
	}
}

func TestRuleMatchHeader(t *testing.T) {
	t.Parallel()

	header := http.Header{
		"X-Token": []string{"secret-123"},
		"X-Other": []string{"secret-123"},
		"Accept":  []string{"text/html"},
		"X-Multi": []string{"nope", "secret-123"},
	}

	tests := []struct {
		name  string
		rule  Rule
		match bool
	}{
		{
			name: "key and value match same header",
			rule: Rule{Header: Header{
				Key:   mustRegexp(t, `^X-Token$`),
				Value: mustRegexp(t, `^secret-`),
			}},
			match: true,
		},
		{
			name: "value matches one of multiple values",
			rule: Rule{Header: Header{
				Key:   mustRegexp(t, `^X-Multi$`),
				Value: mustRegexp(t, `^secret-123$`),
			}},
			match: true,
		},
		{
			name: "key matches but value does not",
			rule: Rule{Header: Header{
				Key:   mustRegexp(t, `^X-Token$`),
				Value: mustRegexp(t, `^nope$`),
			}},
			match: false,
		},
		{
			name: "value matches but key does not",
			rule: Rule{Header: Header{
				Key:   mustRegexp(t, `^X-Absent$`),
				Value: mustRegexp(t, `^secret-`),
			}},
			match: false,
		},
		{
			name: "key and value must match the same header",
			rule: Rule{Header: Header{
				Key:   mustRegexp(t, `^Accept$`),
				Value: mustRegexp(t, `^secret-`),
			}},
			match: false,
		},
		{
			name: "only key set does not match",
			rule: Rule{Header: Header{
				Key: mustRegexp(t, `^X-Token$`),
			}},
			match: false,
		},
		{
			name: "only value set does not match",
			rule: Rule{Header: Header{
				Value: mustRegexp(t, `^secret-`),
			}},
			match: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := newRequest(t, "https://example.com", header)

			if got := tt.rule.Match(req, nil); got != tt.match {
				t.Errorf("expected match=%v, got %v", tt.match, got)
			}
		})
	}
}

func TestRuleMatchBody(t *testing.T) {
	t.Parallel()

	rule := Rule{Body: mustRegexp(t, `password=\w+`)}
	req := newRequest(t, "https://example.com", nil)

	if !rule.Match(req, []byte("user=bob&password=hunter2")) {
		t.Error("expected rule to match request body")
	}

	if rule.Match(req, []byte("user=bob")) {
		t.Error("expected rule to not match request body")
	}
}

func TestMatchBytesLiteralPrefix(t *testing.T) {
	t.Parallel()

	literal := mustRegexp(t, `secret`)
	if !matchBytes(literal, []byte("this contains secret data")) {
		t.Error("expected literal regexp to match")
	}

	if matchBytes(literal, []byte("no match here")) {
		t.Error("expected literal regexp to not match")
	}

	prefixed := mustRegexp(t, `secret-\d+`)
	if !matchBytes(prefixed, []byte("token: secret-42")) {
		t.Error("expected prefixed regexp to match")
	}

	if matchBytes(prefixed, []byte("no prefix present")) {
		t.Error("expected prefixed regexp to not match")
	}

	// A body missing the literal prefix must not match, and the check
	// should not require a full regex scan of the body.
	large := bytes.Repeat([]byte("a"), 1<<20)
	if matchBytes(prefixed, large) {
		t.Error("expected prefixed regexp to not match large body")
	}
}

func TestRuleMarshalUnmarshalBinary(t *testing.T) {
	t.Parallel()

	rule := Rule{
		URL: mustRegexp(t, `^https://example\.com`),
		Header: Header{
			Key:   mustRegexp(t, `^X-Token$`),
			Value: mustRegexp(t, `^secret-`),
		},
		Body: mustRegexp(t, `password=\w+`),
	}

	data, err := rule.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary returned error: %v", err)
	}

	var got Rule
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary returned error: %v", err)
	}

	if got.URL.String() != rule.URL.String() ||
		got.Header.Key.String() != rule.Header.Key.String() ||
		got.Header.Value.String() != rule.Header.Value.String() ||
		got.Body.String() != rule.Body.String() {
		t.Errorf("round-trip mismatch, got %+v", got)
	}
}

func TestRuleUnmarshalBinaryDegradesInvalidPatterns(t *testing.T) {
	t.Parallel()

	dto := ruleDTO{
		URL:  `^https://example\.com`,
		Body: `password=\w+`,
	}
	dto.Header.Key = `^X-Token$`
	dto.Header.Value = `[invalid`

	buf := bytes.Buffer{}
	if err := gob.NewEncoder(&buf).Encode(dto); err != nil {
		t.Fatalf("failed to gob-encode DTO: %v", err)
	}

	var got Rule
	if err := got.UnmarshalBinary(buf.Bytes()); err != nil {
		t.Fatalf("UnmarshalBinary should degrade invalid patterns, got error: %v", err)
	}

	if got.URL == nil || got.Header.Key == nil || got.Body == nil {
		t.Error("expected valid patterns to survive decoding")
	}

	if got.Header.Value != nil {
		t.Error("expected invalid header value pattern to degrade to nil")
	}

	// The degraded rule must still be usable for matching.
	req := newRequest(t, "https://example.com/foo", nil)
	if !got.Match(req, nil) {
		t.Error("expected degraded rule to still match on URL")
	}
}

func TestRuleUnmarshalBinaryCorruptData(t *testing.T) {
	t.Parallel()

	var got Rule
	if err := got.UnmarshalBinary([]byte("not gob data")); err == nil {
		t.Error("expected error for corrupt data")
	}
}

func TestScopeMatch(t *testing.T) {
	t.Parallel()

	s := &Scope{}
	s.SetRules([]Rule{
		{URL: mustRegexp(t, `^https://example\.com`)},
		{Body: mustRegexp(t, `secret`)},
	})

	req := newRequest(t, "https://example.com/foo", nil)
	if !s.Match(req, nil) {
		t.Error("expected scope to match on URL rule")
	}

	req = newRequest(t, "https://other.org", nil)
	if !s.Match(req, []byte("a secret body")) {
		t.Error("expected scope to match on body rule")
	}

	if s.Match(req, []byte("nothing here")) {
		t.Error("expected scope to not match")
	}

	s.SetRules(nil)
	if s.Match(req, nil) {
		t.Error("expected empty scope to not match")
	}
}
