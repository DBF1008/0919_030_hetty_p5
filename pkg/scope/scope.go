package scope

import (
	"bytes"
	"encoding/gob"
	"net/http"
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
	if r.URL != nil {
		if matches := r.URL.MatchString(req.URL.String()); matches {
			return true
		}
	}

	// A header rule is only evaluated when both key and value patterns are
	// set; a partially configured header rule would match too broadly. Both
	// patterns must match the same header.
	if r.Header.Key != nil && r.Header.Value != nil {
		for key, values := range req.Header {
			if matches := r.Header.Key.MatchString(key); !matches {
				continue
			}

			for _, value := range values {
				if matches := r.Header.Value.MatchString(value); matches {
					return true
				}
			}
		}
	}

	if r.Body != nil {
		if matches := matchBytes(r.Body, body); matches {
			return true
		}
	}

	return false
}

// matchBytes reports whether re matches b. It uses the regexp's literal
// prefix (when available) to cheaply reject non-matching input with a
// substring search before running a full regex scan, which avoids scanning
// large bodies with the regex engine when no match is possible.
func matchBytes(re *regexp.Regexp, b []byte) bool {
	prefix, complete := re.LiteralPrefix()
	if complete {
		return bytes.Contains(b, []byte(prefix))
	}

	if prefix != "" && !bytes.Contains(b, []byte(prefix)) {
		return false
	}

	return re.Match(b)
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

	// Compile patterns leniently: an invalid pattern degrades to nil (the
	// criterion is disabled) instead of failing to load the whole rule.
	url, _ := stringToRegexp(dto.URL)
	headerKey, _ := stringToRegexp(dto.Header.Key)
	headerValue, _ := stringToRegexp(dto.Header.Value)
	body, _ := stringToRegexp(dto.Body)

	*r = Rule{
		URL: url,
		Header: Header{
			Key:   headerKey,
			Value: headerValue,
		},
		Body: body,
	}

	return nil
}
