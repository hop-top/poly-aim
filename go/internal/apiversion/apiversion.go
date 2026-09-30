// Package apiversion enforces aim's MAJOR.MINOR capability-negotiation
// contract. Adopters set --api-version on every invocation; the value
// runs through [Negotiate] before any leaf RunE fires. Unsupported
// versions return an [errs.InvalidFlag] envelope so kit's WrapRunE
// middleware renders them to stderr in the active --format.
//
// The shipped contract:
//
//   - [Current] is the version aim emits on every envelope/spec
//     manifest today.
//   - [Supported] is the set of MAJOR.MINOR values aim will honor.
//   - An empty --api-version (default) is a no-op — aim treats it as
//     "I accept whatever's current".
//   - Anything else must appear verbatim in [Supported]; otherwise
//     Negotiate returns an INVALID_API_VERSION envelope.
//
// MINOR bumps are additive (new envelope fields, new spec entries).
// MAJOR bumps remove or change semantics; the deprecation window is
// one MINOR cycle. See docs/schema-changelog.md for the canonical log.
package apiversion

import (
	"fmt"
	"strconv"
	"strings"

	"hop.top/aim/internal/errs"
	"hop.top/kit/go/console/output"
)

const (
	// Current is the MAJOR.MINOR pin aim emits on envelopes and the
	// spec manifest today. Mirrors the constant passed to
	// cli.RegisterSpecCommand in cmd/aim/main.go.
	Current = "1.0"
)

// Supported enumerates the MAJOR.MINOR API versions aim will honor at
// runtime. Add a new entry on every MINOR bump; remove an entry only
// after a full deprecation window per the schema-versioning policy in
// .tlc/tracks/aim-12-factor-conformance/spec.md.
var Supported = []string{"1.0"}

// Negotiate selects the API version aim will honor for this invocation.
// An empty requested value falls back to [Current] (default behaviour
// — caller did not pass --api-version). A non-empty value must appear
// verbatim in [Supported]; anything else surfaces an INVALID_API_VERSION
// envelope keyed by [errs.CodeInvalidFlag] so kit's WrapRunE middleware
// renders it in the active --format.
//
// Negotiate is a pure function: no I/O, no logging, no globals beyond
// reading [Current]/[Supported]. Safe to call from PersistentPreRunE.
func Negotiate(requested string) (string, *output.Error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return Current, nil
	}
	for _, v := range Supported {
		if v == requested {
			return v, nil
		}
	}
	detail := fmt.Sprintf(
		"unsupported api-version: %s. Supported: %s. "+
			"See `aim spec --version` for the current schema version.",
		requested, strings.Join(Supported, ", "))
	return "", errs.InvalidFlag("api-version", requested, detail)
}

// Compatible reports whether requested is honor-able under current —
// same MAJOR, requested MINOR <= current MINOR. Unparseable inputs
// return false so callers fall back to the stricter [Negotiate]-style
// allowlist check. Useful when filtering envelope fields per the
// requested schema (e.g. hide additive fields introduced after the
// caller's pin).
func Compatible(requested, current string) bool {
	reqMaj, reqMin, ok := parse(requested)
	if !ok {
		return false
	}
	curMaj, curMin, ok := parse(current)
	if !ok {
		return false
	}
	if reqMaj != curMaj {
		return false
	}
	return reqMin <= curMin
}

// parse splits a MAJOR.MINOR string into integers. ok=false on any
// shape that isn't exactly two dot-separated non-negative integers.
func parse(s string) (major, minor int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || maj < 0 || min < 0 {
		return 0, 0, false
	}
	return maj, min, true
}
