package ui_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

func TestAssetsServeEmbeddedFilesWithContentTypes(t *testing.T) {
	t.Parallel()

	handler := ui.Assets()
	cases := map[string]string{
		"/ui.css":                       "text/css; charset=utf-8",
		"/components.js":                "text/javascript; charset=utf-8",
		"/manager-page.js":              "text/javascript; charset=utf-8",
		"/aircraft-page.js":             "text/javascript; charset=utf-8",
		"/notices.html":                 "text/html; charset=utf-8",
		"/leaflet/leaflet-src.esm.js":   "text/javascript; charset=utf-8",
		"/leaflet/leaflet.css":          "text/css; charset=utf-8",
		"/leaflet/images/layers.png":    "image/png",
		"/licenses/leaflet-LICENSE.txt": "text/plain; charset=utf-8",
	}
	for path, contentType := range cases {
		recorder := get(t, handler, http.MethodGet, path)
		require.Equal(t, http.StatusOK, recorder.Code, path)
		require.Equal(t, contentType, recorder.Header().Get("Content-Type"), path)
		require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"), path)
		require.NotZero(t, recorder.Body.Len(), path)
	}
}

func TestAssetsRejectDirectoriesUnknownPathsAndTraversal(t *testing.T) {
	t.Parallel()

	handler := ui.Assets()
	for _, path := range []string{
		"/", "/leaflet/", "/leaflet", "/licenses/", "/missing.js", "/README.md",
		"/leaflet/README.md", "/../go.mod", "/static/ui.css", "/templates/manager.html",
		"/ui.css/", "/UI.CSS", "/ui.css?v=1",
	} {
		require.Equal(t, http.StatusNotFound, get(t, handler, http.MethodGet, path).Code, path)
	}
	require.Equal(t, http.StatusMethodNotAllowed, get(t, handler, http.MethodPost, "/ui.css").Code)
}

// TestLeafletAssetsMatchPinnedChecksums keeps the bundled third-party bytes
// equal to the provenance record in static/leaflet/README.md.
func TestLeafletAssetsMatchPinnedChecksums(t *testing.T) {
	t.Parallel()

	pinned := map[string]string{
		"/leaflet/leaflet-src.esm.js":        "39ee93464f11fe3847137e50c0dc8189f706c460e36989ea7871bf7d540f3306",
		"/leaflet/leaflet.css":               "a7837102824184820dfa198d1ebcd109ff6d0ff9a2672a074b9a1b4d147d04c6",
		"/leaflet/images/layers.png":         "1dbbe9d028e292f36fcba8f8b3a28d5e8932754fc2215b9ac69e4cdecf5107c6",
		"/leaflet/images/layers-2x.png":      "066daca850d8ffbef007af00b06eac0015728dee279c51f3cb6c716df7c42edf",
		"/leaflet/images/marker-icon.png":    "574c3a5cca85f4114085b6841596d62f00d7c892c7b03f28cbfa301deb1dc437",
		"/leaflet/images/marker-icon-2x.png": "00179c4c1ee830d3a108412ae0d294f55776cfeb085c60129a39aa6fc4ae2528",
		"/leaflet/images/marker-shadow.png":  "264f5c640339f042dd729062cfc04c17f8ea0f29882b538e3848ed8f10edb4da",
		"/licenses/leaflet-LICENSE.txt":      "53e8dc25862014e4324741ca18fbe3611e11d42ef69f59f86ea8c5389647d4cb",
	}
	for path, want := range pinned {
		recorder := get(t, ui.Assets(), http.MethodGet, path)
		require.Equal(t, http.StatusOK, recorder.Code, path)
		sum := sha256.Sum256(recorder.Body.Bytes())
		require.Equal(t, want, hex.EncodeToString(sum[:]), path)
	}
}

func TestNoticesLinkEveryBundledLicense(t *testing.T) {
	t.Parallel()

	notices := get(t, ui.Assets(), http.MethodGet, "/notices.html").Body.String()
	require.Contains(t, notices, `href="licenses/leaflet-LICENSE.txt"`)
	require.Contains(t, notices, "1.9.4")
}
