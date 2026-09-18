// Package urlpath validates public URLs used to address mounted handlers.
//
// It implements source base URL validation: an absolute http or https URL
// with a preserved mount prefix and exactly one trailing slash, so a client
// can append a relative route path without losing or duplicating a segment.
// Errors satisfy errors.Is with ErrInvalid.
//
// Route prefix, API base, and asset path validation for the browser UI remain
// planned.
package urlpath
