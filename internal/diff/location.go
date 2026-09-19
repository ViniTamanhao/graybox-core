package diff

import (
	"strings"
)

// Component identifies the portion of an HTTP response being compared.
type Component string

const (
	// ComponentStatus identifies the HTTP response status.
	ComponentStatus Component = "response.status"

	// ComponentHeaders identifies response headers.
	ComponentHeaders Component = "response.headers"

	// ComponentBody identifies the response body.
	ComponentBody Component = "response.body"
)

// Location identifies where a behavioral difference occurred.
//
// Path has component-specific meaning:
//
//   - response.status: Path is empty.
//   - response.headers: Path is the lower-case header name.
//   - response.body: Path is an RFC 6901 JSON Pointer relative to the body.
//     The empty string represents the whole body.
//
// Examples:
//
//	Location{Component: ComponentStatus}
//	Location{Component: ComponentHeaders, Path: "content-type"}
//	Location{Component: ComponentBody, Path: "/user/name"}
//	Location{Component: ComponentBody, Path: "/items/0/price"}
//
// Keeping Component and Path separate avoids making the comparison engine
// parse presentation-oriented dotted strings.
type Location struct {
	Component Component
	Path      string
}

// StatusLocation returns the canonical response-status location.
func StatusLocation() Location {
	return Location{
		Component: ComponentStatus,
	}
}

// HeaderLocation returns the canonical location for a response header.
//
// Header names are case-insensitive according to HTTP semantics, so Graybox
// normalizes them to lower case for comparison paths and ignore rules.
func HeaderLocation(name string) Location {
	return Location{
		Component: ComponentHeaders,
		Path:      strings.ToLower(strings.TrimSpace(name)),
	}
}

// BodyLocation returns a response-body location.
//
// pointer is an RFC 6901 JSON Pointer relative to the response body. The empty
// pointer identifies the body as a whole.
func BodyLocation(pointer string) Location {
	return Location{
		Component: ComponentBody,
		Path:      pointer,
	}
}

// String returns a stable textual representation useful for diagnostics.
//
// Renderers are free to produce a friendlier representation for humans.
func (l Location) String() string {
	if l.Path == "" {
		return string(l.Component)
	}

	switch l.Component {
	case ComponentHeaders:
		return string(l.Component) + "." + l.Path
	case ComponentBody:
		return string(l.Component) + "#" + l.Path
	default:
		return string(l.Component) + "." + l.Path
	}
}

// AppendJSONPointer appends one decoded JSON object key or array index token to
// an RFC 6901 JSON Pointer.
//
// The token is escaped here so callers comparing JSON do not have to duplicate
// pointer escaping rules.
func AppendJSONPointer(pointer, token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")

	if pointer == "" {
		return "/" + token
	}

	return pointer + "/" + token
}

// Rules contains semantic comparison rules.
//
// V1 deliberately starts with ignore locations only. Numeric tolerances,
// unordered arrays, regex matching, schemas, and custom comparison scripts can
// be added later if actual usage demonstrates the need.
type Rules struct {
	Ignore []Location
}

// Ignores reports whether a location should be excluded from comparison.
//
// Ignoring the empty path for a component ignores that entire component:
//
//	Location{Component: ComponentHeaders}
//
// ignores all response headers.
//
// For JSON bodies, ignoring a location also ignores its descendants. Ignoring:
//
//	/metadata
//
// therefore also ignores:
//
//	/metadata/request_id
//	/metadata/created_at
//
// Header rules are case-insensitive.
func (r Rules) Ignores(location Location) bool {
	for _, ignored := range r.Ignore {
		if ignored.Component != location.Component {
			continue
		}

		if ignored.Path == "" {
			return true
		}

		switch location.Component {
		case ComponentHeaders:
			if strings.EqualFold(ignored.Path, location.Path) {
				return true
			}

		case ComponentBody:
			if ignored.Path == location.Path {
				return true
			}

			if strings.HasPrefix(location.Path, ignored.Path+"/") {
				return true
			}

		default:
			if ignored.Path == location.Path {
				return true
			}
		}
	}

	return false
}
