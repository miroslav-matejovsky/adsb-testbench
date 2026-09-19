package ui_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

var configElement = regexp.MustCompile(`(?s)<script type="application/json" id="tb-config">(.*?)</script>`)

func get(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

// renderedConfig extracts and decodes the page's JSON configuration.
func renderedConfig(t *testing.T, body string) map[string]any {
	t.Helper()

	match := configElement.FindStringSubmatch(body)
	require.Len(t, match, 2, "the page carries one JSON configuration element")
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(match[1]), &config))
	return config
}

func TestManagerPageRendersExplicitConfiguration(t *testing.T) {
	t.Parallel()

	config := managerConfig()
	config.APIBaseURL = "/bench/a/api/simulator"
	config.AssetBaseURL = "/bench/a/assets"
	handler, err := ui.NewManager(config)
	require.NoError(t, err)

	recorder := get(t, handler, http.MethodGet, "/")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/html; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Contains(t, recorder.Header().Get("Content-Security-Policy"), "script-src 'self'")
	body := recorder.Body.String()
	require.Contains(t, body, `href="/bench/a/assets/ui.css"`)
	require.Contains(t, body, `src="/bench/a/assets/manager-page.js"`)
	require.Contains(t, body, `href="/bench/a/assets/notices.html"`)
	require.Contains(t, body, `data-tb-mount="manager"`)
	require.Equal(t, map[string]any{
		"apiBaseUrl": "/bench/a/api/simulator/", "assetBaseUrl": "/bench/a/assets/",
		"pollIntervalMilliseconds": 1000.0, "requestTimeoutMilliseconds": 5000.0,
		"maxResponseBytes": 1048576.0, "resumeSpeedHundredths": 100.0,
	}, renderedConfig(t, body))
}

func TestAircraftPageRendersExplicitConfiguration(t *testing.T) {
	t.Parallel()

	config := aircraftConfig()
	config.Tiles = tiles()
	handler, err := ui.NewAircraftDisplay(config)
	require.NoError(t, err)

	recorder := get(t, handler, http.MethodGet, "/")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Security-Policy"), "img-src 'self' data: https://tiles.example.test")
	body := recorder.Body.String()
	require.Contains(t, body, `href="/assets/leaflet/leaflet.css"`)
	require.Contains(t, body, `src="/assets/aircraft-page.js"`)
	rendered := renderedConfig(t, body)
	require.Equal(t, "10000000000", rendered["freshForNanoseconds"])
	require.Equal(t, "60000000000", rendered["lostAfterNanoseconds"])
	require.Equal(t, []any{"alpha"}, rendered["stationIds"])
	require.Equal(t, map[string]any{
		"urlTemplate": "https://tiles.example.test/{z}/{x}/{y}.png", "attributionText": "Example tiles",
		"attributionUrl": "https://tiles.example.test/about", "minZoom": 0.0, "maxZoom": 18.0,
	}, rendered["tiles"])

	withoutTiles, err := ui.NewAircraftDisplay(aircraftConfig())
	require.NoError(t, err)
	recorder = get(t, withoutTiles, http.MethodGet, "/")
	require.Nil(t, renderedConfig(t, recorder.Body.String())["tiles"])
	require.NotContains(t, recorder.Header().Get("Content-Security-Policy"), "tiles.example.test")
}

func TestPagesEscapeHostileConfigurationText(t *testing.T) {
	t.Parallel()

	config := aircraftConfig()
	config.Tiles = tiles()
	config.Tiles.AttributionText = `</script><script>alert("x")</script> & <b>`
	handler, err := ui.NewAircraftDisplay(config)
	require.NoError(t, err)
	body := get(t, handler, http.MethodGet, "/").Body.String()

	require.NotContains(t, body, `<script>alert`)
	require.Equal(t, 2, strings.Count(body, "</script>"), "only the module and config elements close scripts")
	rendered := renderedConfig(t, body)
	require.Equal(t, config.Tiles.AttributionText, rendered["tiles"].(map[string]any)["attributionText"])
}

func TestPagesServeOnlyTheirRoot(t *testing.T) {
	t.Parallel()

	handler, err := ui.NewManager(managerConfig())
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, get(t, handler, http.MethodGet, "/other").Code)
	require.Equal(t, http.StatusMethodNotAllowed, get(t, handler, http.MethodPost, "/").Code)
	head := get(t, handler, http.MethodHead, "/")
	require.Equal(t, http.StatusOK, head.Code)
	require.Zero(t, head.Body.Len())
}

func TestPagesMountBelowNestedPrefixes(t *testing.T) {
	t.Parallel()

	manager, err := ui.NewManager(managerConfig())
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle("/bench/a/manager/", http.StripPrefix("/bench/a/manager", manager))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/bench/a/manager/", nil)
	require.NoError(t, err)
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, string(body), "Simulator manager")
}

func TestPageConstructorsRejectInvalidConfiguration(t *testing.T) {
	t.Parallel()

	manager := managerConfig()
	manager.APIBaseURL = "//evil.test/"
	_, err := ui.NewManager(manager)
	require.ErrorIs(t, err, ui.ErrInvalidConfig)

	aircraft := aircraftConfig()
	aircraft.StationIDs = nil
	_, err = ui.NewAircraftDisplay(aircraft)
	require.ErrorIs(t, err, ui.ErrInvalidConfig)
}
