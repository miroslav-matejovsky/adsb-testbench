package urlpath

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalid identifies a URL outside the accepted domain of this package.
var ErrInvalid = errors.New("invalid URL")

// ParseSourceBase validates an absolute http or https base URL that a client
// appends relative route paths to, and returns it with exactly one trailing
// slash.
//
// The whole mount prefix is preserved, so "https://host/bench/a/simulator"
// and "https://host/bench/a/simulator/" both become
// "https://host/bench/a/simulator/" and appending "receptions/history"
// addresses the mounted route rather than the server root.
//
// Userinfo, queries, fragments, empty path segments, dot segments, and
// percent-encoded path separators are rejected. Each of them would make the
// appended route resolve somewhere the caller did not name.
func ParseSourceBase(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: base URL is empty", ErrInvalid)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: parse base URL %q: %w", ErrInvalid, raw, err)
	}
	switch {
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		return "", fmt.Errorf("%w: base URL %q must use http or https", ErrInvalid, raw)
	case parsed.Host == "":
		return "", fmt.Errorf("%w: base URL %q has no host", ErrInvalid, raw)
	case parsed.User != nil:
		return "", fmt.Errorf("%w: base URL %q must not carry userinfo", ErrInvalid, raw)
	case parsed.RawQuery != "" || parsed.ForceQuery:
		return "", fmt.Errorf("%w: base URL %q must not carry a query", ErrInvalid, raw)
	case parsed.Fragment != "":
		return "", fmt.Errorf("%w: base URL %q must not carry a fragment", ErrInvalid, raw)
	case parsed.Opaque != "":
		return "", fmt.Errorf("%w: base URL %q is not hierarchical", ErrInvalid, raw)
	}
	if err := validBasePath(raw, parsed); err != nil {
		return "", err
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/"
	parsed.RawPath = ""
	return parsed.String(), nil
}

// validBasePath rejects path syntax that would change which route an appended
// relative path resolves to.
func validBasePath(raw string, parsed *url.URL) error {
	escaped := parsed.EscapedPath()
	if strings.Contains(escaped, "//") {
		return fmt.Errorf("%w: base URL %q has an empty path segment", ErrInvalid, raw)
	}
	if strings.Contains(strings.ToUpper(escaped), "%2F") {
		return fmt.Errorf("%w: base URL %q encodes a path separator", ErrInvalid, raw)
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("%w: base URL %q has a dot segment", ErrInvalid, raw)
		}
	}
	return nil
}
