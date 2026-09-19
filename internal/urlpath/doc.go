// Package urlpath validates public URLs and path prefixes used to address
// mounted handlers.
//
// ParseSourceBase validates an absolute http or https URL that a Go client
// appends relative routes to. ParseMountPrefix validates the path prefix a
// host mounts an application under. ParseBrowserBase validates an API or
// asset base used by browser code: either a root-relative path or an absolute
// http or https URL.
//
// Every parser preserves the whole mount path and returns it with exactly one
// trailing slash, so appending "truth" (never "/truth") addresses the mounted
// route. Each rejects syntax that a URL parser, browser, or proxy could
// normalize into a different route: userinfo, query or fragment delimiters,
// empty segments, dot segments, control bytes, backslashes, and
// percent-encoded separators, dot segments or control bytes. Errors satisfy
// errors.Is with ErrInvalid.
package urlpath
