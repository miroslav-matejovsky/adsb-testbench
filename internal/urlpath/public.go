package urlpath

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseMountPrefix validates the public path prefix a host mounts an
// application under and returns it with exactly one trailing slash.
//
// The prefix is "/" or an absolute path such as "/bench/a/". A missing
// trailing slash is added, so "/bench/a" becomes "/bench/a/". Relative paths,
// protocol-relative paths ("//host"), schemes, query or fragment delimiters,
// empty segments, dot segments, control bytes, backslashes, and
// percent-encoded separators or dot segments are rejected. A browser or proxy
// can normalize each of them into a different path than the one configured.
func ParseMountPrefix(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: mount prefix is empty", ErrInvalid)
	}
	if !strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: mount prefix %q must start with /", ErrInvalid, raw)
	}
	if err := validPathText(raw, raw); err != nil {
		return "", err
	}
	return strings.TrimSuffix(raw, "/") + "/", nil
}

// ParseBrowserBase validates a base URL that browser code appends relative
// API or asset routes to, and returns it with exactly one trailing slash.
//
// Two forms are accepted: a root-relative path following the ParseMountPrefix
// rules, or an absolute http or https URL following the ParseSourceBase rules.
// Browser code must still check at mount time that an absolute base resolves
// to the page's own origin.
func ParseBrowserBase(raw string) (string, error) {
	if strings.HasPrefix(raw, "/") {
		return ParseMountPrefix(raw)
	}
	return ParseSourceBase(raw)
}

// validPathText rejects path syntax that a URL parser, browser, or proxy could
// resolve to a different route than the literal configured path. path is the
// escaped path text; raw is the whole input used in error messages.
func validPathText(raw, path string) error {
	for i := range len(raw) {
		b := raw[i]
		if b < 0x20 || b == 0x7f {
			return fmt.Errorf("%w: %q contains a control byte", ErrInvalid, raw)
		}
	}
	switch {
	case strings.ContainsAny(raw, `\`):
		return fmt.Errorf("%w: %q contains a backslash", ErrInvalid, raw)
	case strings.ContainsAny(raw, "?#"):
		return fmt.Errorf("%w: %q contains a query or fragment delimiter", ErrInvalid, raw)
	case strings.Contains(path, "//"):
		return fmt.Errorf("%w: %q has an empty path segment", ErrInvalid, raw)
	}
	for segment := range strings.SplitSeq(path, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return fmt.Errorf("%w: %q has a malformed escape: %w", ErrInvalid, raw, err)
		}
		switch {
		case decoded != segment && strings.ContainsAny(decoded, `/\`):
			return fmt.Errorf("%w: %q encodes a path separator", ErrInvalid, raw)
		case decoded == "." || decoded == "..":
			return fmt.Errorf("%w: %q has a dot segment", ErrInvalid, raw)
		}
		for i := range len(decoded) {
			if decoded[i] < 0x20 || decoded[i] == 0x7f {
				return fmt.Errorf("%w: %q encodes a control byte", ErrInvalid, raw)
			}
		}
	}
	return nil
}
