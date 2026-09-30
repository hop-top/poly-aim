package aim

import (
	"fmt"
	"strings"
)

// knownTagKeys is the set of recognised tag key names.
var knownTagKeys = map[string]struct{}{
	"in":                {},
	"out":               {},
	"provider":          {},
	"family":            {},
	"tool_call":         {},
	"reasoning":         {},
	"open_weights":      {},
	"structured_output": {},
	"temperature":       {},
}

// Explanation is the structured form returned by [ExplainQuery]. It
// surfaces both the parsed Filter and the free-text token slice so
// agents can validate DSL shape before running a query.
type Explanation struct {
	// Input is the raw expression string supplied by the caller,
	// trimmed of leading/trailing whitespace.
	Input string
	// Filter is the parsed Filter. Tristate fields stay nil when the
	// corresponding tag was absent in the input.
	Filter Filter
	// FreeText is the slice of bare/quoted tokens collected during
	// parsing, in order. The same content is also joined into
	// Filter.Query — this slice preserves the per-token boundary.
	FreeText []string
}

// ExplainQuery parses q exactly like [ParseQuery] but returns the parsed
// Filter together with the per-token free-text slice. It performs no
// registry lookups; pure parser surface.
func ExplainQuery(q string) (Explanation, error) {
	f, freeText, err := parseQueryInternal(q)
	if err != nil {
		return Explanation{}, err
	}
	return Explanation{
		Input:    strings.TrimSpace(q),
		Filter:   f,
		FreeText: freeText,
	}, nil
}

// ParseQuery parses a string query into a Filter.
// Returns error for unknown tag keys, invalid bool values, or bare colons.
//
// Syntax:
//   - key:value — structured tag (see known keys)
//   - "..." — quoted free-text; colons inside are literal
//   - bare token without colon — free-text appended to Filter.Query
func ParseQuery(q string) (Filter, error) {
	f, _, err := parseQueryInternal(q)
	return f, err
}

// parseQueryInternal implements the parser body shared by ParseQuery
// and ExplainQuery. It returns both the Filter and the ordered slice of
// free-text tokens (callers that don't need the slice ignore it).
func parseQueryInternal(q string) (Filter, []string, error) {
	var f Filter
	var freeText []string

	tokens, err := tokenise(q)
	if err != nil {
		return Filter{}, nil, err
	}

	for _, tok := range tokens {
		if tok.quoted {
			freeText = append(freeText, tok.val)
			continue
		}

		colonIdx := strings.Index(tok.val, ":")
		if colonIdx == -1 {
			// plain free-text
			freeText = append(freeText, tok.val)
			continue
		}

		key := tok.val[:colonIdx]
		val := tok.val[colonIdx+1:]

		if key == "" || val == "" {
			return Filter{}, nil, fmt.Errorf("aim: malformed tag %q: key and value must both be non-empty", tok.val)
		}

		if _, ok := knownTagKeys[key]; !ok {
			return Filter{}, nil, fmt.Errorf("aim: unknown tag key %q", key)
		}

		if err := applyTag(&f, key, val); err != nil {
			return Filter{}, nil, err
		}
	}

	if len(freeText) > 0 {
		f.Query = strings.Join(freeText, " ")
	}

	return f, freeText, nil
}

// applyTag sets the corresponding Filter field for a known key:value pair.
func applyTag(f *Filter, key, val string) error {
	switch key {
	case "in":
		parts := strings.Split(val, ",")
		f.Input = append(f.Input, parts...)
	case "out":
		parts := strings.Split(val, ",")
		f.Output = append(f.Output, parts...)
	case "provider":
		f.Provider = val
	case "family":
		f.Family = val
	case "tool_call":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("aim: tag tool_call: %w", err)
		}
		f.ToolCall = &b
	case "reasoning":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("aim: tag reasoning: %w", err)
		}
		f.Reasoning = &b
	case "open_weights":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("aim: tag open_weights: %w", err)
		}
		f.OpenWeights = &b
	case "structured_output":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("aim: tag structured_output: %w", err)
		}
		f.StructuredOutput = &b
	case "temperature":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("aim: tag temperature: %w", err)
		}
		f.Temperature = &b
	}
	return nil
}

// parseBool accepts only "true" or "false".
func parseBool(s string) (bool, error) {
	switch s {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool value %q: must be \"true\" or \"false\"", s)
	}
}

// token is a single lexed unit from the query string.
type token struct {
	val    string
	quoted bool
}

// tokenise splits the query into tokens, respecting double-quoted strings.
// Quoted content is returned with quoted=true and the surrounding quotes stripped.
// Returns an error for unterminated quotes.
func tokenise(q string) ([]token, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}

	var tokens []token
	i := 0
	n := len(q)

	for i < n {
		// skip whitespace
		if q[i] == ' ' || q[i] == '\t' {
			i++
			continue
		}

		if q[i] == '"' {
			// quoted string
			j := i + 1
			for j < n && q[j] != '"' {
				j++
			}
			if j >= n {
				return nil, fmt.Errorf("aim: unterminated quoted string in query")
			}
			tokens = append(tokens, token{val: q[i+1 : j], quoted: true})
			i = j + 1
			continue
		}

		// unquoted token — read until whitespace
		j := i
		for j < n && q[j] != ' ' && q[j] != '\t' && q[j] != '"' {
			j++
		}
		raw := q[i:j]
		i = j

		// bare colon check
		if raw == ":" {
			return nil, fmt.Errorf("aim: bare colon in query")
		}

		tokens = append(tokens, token{val: raw, quoted: false})
	}

	return tokens, nil
}
