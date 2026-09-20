package scope

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"net/url"
	"regexp"
	"sync"
)

type Scope struct {
	rules []Rule
	mu    sync.RWMutex
}

type Rule struct {
	URL    *regexp.Regexp
	Header Header
	Body   *regexp.Regexp
}

type Header struct {
	Key   *regexp.Regexp
	Value *regexp.Regexp
}

func (s *Scope) Rules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.rules
}

func (s *Scope) SetRules(rules []Rule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rules = rules
}

func (s *Scope) Match(req *http.Request, body []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rule := range s.rules {
		if matches := rule.Match(req, body); matches {
			return true
		}
	}

	return false
}

func (r Rule) Match(req *http.Request, body []byte) bool {
	return r.MatchesURL(req.URL) || r.MatchesHeader(req.Header) || r.MatchesBody(body)
}

// MatchesURL reports whether the rule's URL criterion matches the given URL.
func (r Rule) MatchesURL(u *url.URL) bool {
	if r.URL == nil || u == nil {
		return false
	}

	return r.URL.MatchString(u.String())
}

// MatchesHeader reports whether the rule's header criterion matches the given
// HTTP headers. Both a key and a value pattern must be configured for the
// header criterion to be effective, and both must match the *same* header.
// A partially configured header criterion (only a key or only a value
// pattern) never matches; this prevents overly broad rules that would match
// any request carrying a common header.
func (r Rule) MatchesHeader(header http.Header) bool {
	if r.Header.Key == nil || r.Header.Value == nil {
		return false
	}

	for key, values := range header {
		if !r.Header.Key.MatchString(key) {
			continue
		}

		for _, value := range values {
			if r.Header.Value.MatchString(value) {
				return true
			}
		}
	}

	return false
}

// MatchesBody reports whether the rule's body criterion matches the given
// body. To avoid scanning large bodies with the regexp engine, a cheap
// literal-prefix prefilter is used to reject bodies that cannot match.
func (r Rule) MatchesBody(body []byte) bool {
	if r.Body == nil || len(body) == 0 {
		return false
	}

	prefix, complete := r.Body.LiteralPrefix()
	if complete {
		// The regexp is a literal string; matching boils down to a
		// substring search.
		return bytes.Contains(body, []byte(prefix))
	}

	if prefix != "" && !bytes.Contains(body, []byte(prefix)) {
		return false
	}

	return r.Body.Match(body)
}

func regexpToString(r *regexp.Regexp) string {
	if r == nil {
		return ""
	}

	return r.String()
}

func stringToRegexp(s string) (*regexp.Regexp, error) {
	if s == "" {
		return nil, nil
	}

	return regexp.Compile(s)
}

// compileRegexpLenient compiles the given pattern, returning nil if the
// pattern is empty or invalid. It is used when loading persisted rules, so
// that a single invalid pattern doesn't prevent the whole rule (and thus the
// project) from being loaded.
func compileRegexpLenient(s string) *regexp.Regexp {
	re, err := stringToRegexp(s)
	if err != nil {
		return nil
	}

	return re
}

type ruleDTO struct {
	URL    string
	Header struct {
		Key   string
		Value string
	}
	Body string
}

func (r Rule) MarshalBinary() ([]byte, error) {
	dto := ruleDTO{
		URL:  regexpToString(r.URL),
		Body: regexpToString(r.Body),
	}
	dto.Header.Key = regexpToString(r.Header.Key)
	dto.Header.Value = regexpToString(r.Header.Value)

	buf := bytes.Buffer{}

	err := gob.NewEncoder(&buf).Encode(dto)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (r *Rule) UnmarshalBinary(data []byte) error {
	dto := ruleDTO{}

	err := gob.NewDecoder(bytes.NewReader(data)).Decode(&dto)
	if err != nil {
		return err
	}

	// Compile patterns leniently: if a stored pattern fails to compile
	// (e.g. data written by an incompatible version), degrade by dropping
	// that criterion instead of failing to load the entire rule set.
	*r = Rule{
		URL: compileRegexpLenient(dto.URL),
		Header: Header{
			Key:   compileRegexpLenient(dto.Header.Key),
			Value: compileRegexpLenient(dto.Header.Value),
		},
		Body: compileRegexpLenient(dto.Body),
	}

	return nil
}
