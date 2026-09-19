package display

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// stubProvider is an in-process provider returning fixed fixture data.
type stubProvider struct {
	snapshot     simulatorapi.ReceptionSnapshot
	page         simulatorapi.ReceptionPage
	err          error
	seenStations []string
	calls        int
}

func (p *stubProvider) ReceptionSnapshot(_ context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
	p.calls++
	p.seenStations = request.StationIDs
	if p.err != nil {
		return simulatorapi.ReceptionSnapshot{}, p.err
	}
	return p.snapshot, nil
}

func (p *stubProvider) ReceptionHistory(_ context.Context, _ simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	p.calls++
	if p.err != nil {
		return simulatorapi.ReceptionPage{}, p.err
	}
	return p.page, nil
}

// roundTripper serves canned responses and records the requests it saw.
type roundTripper struct {
	mu       sync.Mutex
	requests []*http.Request
	respond  func(*http.Request) (*http.Response, error)
}

func (t *roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.requests = append(t.requests, request)
	t.mu.Unlock()
	return t.respond(request)
}

// countingBody records whether a response body was closed.
type countingBody struct {
	reader io.Reader
	closed int
	err    error
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	return b.reader.Read(p)
}

func (b *countingBody) Close() error {
	b.closed++
	return nil
}

// jsonResponse builds a 200 JSON response with an honest length.
func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Header:        http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:          &countingBody{reader: strings.NewReader(body)},
		ContentLength: int64(len(body)),
	}
}

const sourceBaseURL = "http://simulator.test/bench/a/simulator"

// newHTTPSourceFor returns a source whose transport is fully controlled.
func newHTTPSourceFor(t *testing.T, respond func(*http.Request) (*http.Response, error)) (*HTTPSource, *roundTripper) {
	t.Helper()

	transport := &roundTripper{respond: respond}
	source, err := NewHTTPSource(HTTPSourceConfig{
		BaseURL: sourceBaseURL, Timeout: 5 * time.Second,
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
		Client: &http.Client{Transport: transport},
	})
	require.NoError(t, err)
	return source, transport
}

func encodeFixture(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

func TestBothSourcesReturnEquivalentSnapshots(t *testing.T) {
	t.Parallel()

	fixture := validFixtureSnapshot(t)
	request := simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "bravo"}}

	local, err := NewInProcessSource(&stubProvider{snapshot: fixture})
	require.NoError(t, err)
	fromLocal, err := local.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)

	remote, transport := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, encodeFixture(t, fixture)), nil
	})
	fromHTTP, err := remote.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)

	require.Equal(t, fixture, fromLocal)
	require.Equal(t, fromLocal, fromHTTP)
	require.Equal(t, "/bench/a/simulator/observations/receptions", transport.requests[0].URL.Path)
	require.Equal(t, "application/json", transport.requests[0].Header.Get("Content-Type"))
}

func TestBothSourcesReturnEquivalentHistoryPages(t *testing.T) {
	t.Parallel()

	request, fixture := fixturePage(t)

	local, err := NewInProcessSource(&stubProvider{page: fixture})
	require.NoError(t, err)
	fromLocal, err := local.ReceptionHistory(t.Context(), request)
	require.NoError(t, err)

	remote, transport := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, encodeFixture(t, fixture)), nil
	})
	fromHTTP, err := remote.ReceptionHistory(t.Context(), request)
	require.NoError(t, err)

	require.Equal(t, fixture, fromLocal)
	require.Equal(t, fromLocal, fromHTTP)
	require.Equal(t, "/bench/a/simulator/receptions/history", transport.requests[0].URL.Path)
}

func TestBothSourcesRejectInvalidRequestsBeforeAccess(t *testing.T) {
	t.Parallel()

	provider := &stubProvider{snapshot: validFixtureSnapshot(t)}
	local, err := NewInProcessSource(provider)
	require.NoError(t, err)

	remote, transport := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("an invalid request must not reach the source")
		return nil, nil
	})

	invalid := simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"bad id"}}
	_, err = local.ReceptionSnapshot(t.Context(), invalid)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
	_, err = remote.ReceptionSnapshot(t.Context(), invalid)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)

	require.Zero(t, provider.calls)
	require.Empty(t, transport.requests)
}

func TestInProcessSourcePreservesProviderErrorIdentity(t *testing.T) {
	t.Parallel()

	cause := simulatorapi.WrapError(simulatorapi.CategoryNotFound,
		errors.New("station missing"), "station \"alpha\"").WithRunID(fixtureRunID)
	source, err := NewInProcessSource(&stubProvider{err: cause})
	require.NoError(t, err)

	_, err = source.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.ErrorIs(t, err, simulatorapi.CategoryNotFound)
	require.ErrorIs(t, err, cause)

	var failure *SourceError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, snapshotOperation, failure.Operation)
	require.Equal(t, fixtureRunID, failure.RunID)
}

func TestInProcessSourceRejectsInvalidProviderResponses(t *testing.T) {
	t.Parallel()

	fixture := validFixtureSnapshot(t)
	fixture.Records[0].Frame = "8D48"
	source, err := NewInProcessSource(&stubProvider{snapshot: fixture})
	require.NoError(t, err)

	_, err = source.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "bravo"}})
	require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)

	var failure *SourceError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, fixtureRunID, failure.RunID,
		"a validated envelope still reports its run identity")
}

func TestInProcessSourceRejectsNonFiniteLocalValues(t *testing.T) {
	t.Parallel()

	fixture := validFixtureSnapshot(t)
	fixture.Records[0].Receiver.AntennaGainDBi = math.NaN()
	source, err := NewInProcessSource(&stubProvider{snapshot: fixture})
	require.NoError(t, err)

	_, err = source.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "bravo"}})
	require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)
}

func TestNewInProcessSourceRequiresAProvider(t *testing.T) {
	t.Parallel()

	_, err := NewInProcessSource(nil)
	require.ErrorContains(t, err, "provider is nil")
}

func TestHTTPSourceRejectsMalformedResponses(t *testing.T) {
	t.Parallel()

	fixture := validFixtureSnapshot(t)
	encoded := encodeFixture(t, fixture)

	cases := []struct {
		name     string
		response func() *http.Response
		category simulatorapi.Category
	}{
		{
			name:     "malformed json",
			response: func() *http.Response { return jsonResponse(http.StatusOK, `{"runId":`) },
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name:     "trailing json",
			response: func() *http.Response { return jsonResponse(http.StatusOK, encoded+`{}`) },
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name: "missing key",
			response: func() *http.Response {
				return jsonResponse(http.StatusOK, strings.Replace(encoded, `"now":`, `"then":`, 1))
			},
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name: "duplicate key",
			response: func() *http.Response {
				return jsonResponse(http.StatusOK, strings.Replace(encoded, `{"runId"`, `{"runId":"x","runId"`, 1))
			},
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name: "unknown key",
			response: func() *http.Response {
				return jsonResponse(http.StatusOK, strings.Replace(encoded, `{"runId"`, `{"extra":1,"runId"`, 1))
			},
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name: "wrong media type",
			response: func() *http.Response {
				response := jsonResponse(http.StatusOK, encoded)
				response.Header.Set("Content-Type", "text/plain")
				return response
			},
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name: "missing media type",
			response: func() *http.Response {
				response := jsonResponse(http.StatusOK, encoded)
				response.Header.Del("Content-Type")
				return response
			},
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name: "content encoding",
			response: func() *http.Response {
				response := jsonResponse(http.StatusOK, encoded)
				response.Header.Set("Content-Encoding", "gzip")
				return response
			},
			category: simulatorapi.CategorySourceInvalid,
		},
		{
			name: "semantically invalid payload",
			response: func() *http.Response {
				broken := validFixtureSnapshot(t)
				broken.Retention[0].LatestSequence = "99"
				return jsonResponse(http.StatusOK, encodeFixture(t, broken))
			},
			category: simulatorapi.CategorySourceInvalid,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
				return tc.response(), nil
			})
			_, err := source.ReceptionSnapshot(t.Context(),
				simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "bravo"}})
			require.ErrorIs(t, err, tc.category)
		})
	}
}

func TestHTTPSourceRecreatesRemoteErrorCategories(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		status   int
		code     string
		category simulatorapi.Category
	}{
		{name: "invalid", status: http.StatusBadRequest, code: "invalid", category: simulatorapi.CategoryInvalid},
		{name: "not found", status: http.StatusNotFound, code: "not_found", category: simulatorapi.CategoryNotFound},
		{name: "conflict", status: http.StatusConflict, code: "conflict", category: simulatorapi.CategoryConflict},
		{name: "limit", status: http.StatusUnprocessableEntity, code: "limit", category: simulatorapi.CategoryLimit},
		{name: "unavailable", status: http.StatusServiceUnavailable, code: "unavailable", category: simulatorapi.CategoryUnavailable},
		{name: "deadline", status: http.StatusGatewayTimeout, code: "deadline_exceeded", category: simulatorapi.CategoryDeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := fmt.Sprintf(`{"error":{"code":%q,"message":"upstream said no","field":"","runId":%q}}`,
				tc.code, fixtureRunID)
			source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(tc.status, body), nil
			})

			_, err := source.ReceptionSnapshot(t.Context(),
				simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
			require.ErrorIs(t, err, tc.category)

			var failure *SourceError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, tc.status, failure.Status)
			require.Equal(t, fixtureRunID, failure.RunID)
			require.Equal(t, "upstream said no", failure.Message)
		})
	}
}

func TestHTTPSourceRejectsInconsistentErrorEnvelopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "status contradicts code", status: http.StatusNotFound, body: `{"error":{"code":"invalid","message":"m","field":"","runId":""}}`},
		{name: "unknown code", status: http.StatusBadRequest, body: `{"error":{"code":"teapot","message":"m","field":"","runId":""}}`},
		{name: "missing key", status: http.StatusBadRequest, body: `{"error":{"code":"invalid","message":"m"}}`},
		{name: "not an envelope", status: http.StatusBadRequest, body: `{"message":"m"}`},
		{name: "unreadable", status: http.StatusBadRequest, body: `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(tc.status, tc.body), nil
			})
			_, err := source.ReceptionSnapshot(t.Context(),
				simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
			require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)

			var failure *SourceError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, tc.status, failure.Status)
		})
	}
}

func TestHTTPSourceBoundsResponseBytesDespiteDishonestLength(t *testing.T) {
	t.Parallel()

	oversized := `{"runId":"` + strings.Repeat("x", 6000) + `"}`

	cases := []struct {
		name   string
		length int64
	}{
		{name: "declared length is honest", length: int64(len(oversized))},
		{name: "declared length lies", length: 10},
		{name: "declared length is unknown", length: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
				response := jsonResponse(http.StatusOK, oversized)
				response.ContentLength = tc.length
				return response, nil
			}}
			source, err := NewHTTPSource(HTTPSourceConfig{
				BaseURL: sourceBaseURL, Timeout: time.Second,
				MaxRequestBytes: 4096, MaxResponseBytes: simulatorapi.MinResponseBytes,
				Client: &http.Client{Transport: transport},
			})
			require.NoError(t, err)

			_, err = source.ReceptionSnapshot(t.Context(),
				simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
			require.ErrorIs(t, err, simulatorapi.CategoryResponseLimit)
		})
	}
}

func TestHTTPSourceClosesEveryResponseBody(t *testing.T) {
	t.Parallel()

	fixture := validFixtureSnapshot(t)
	bodies := []*countingBody{}

	cases := []struct {
		name     string
		response func() *http.Response
	}{
		{name: "success", response: func() *http.Response {
			return jsonResponse(http.StatusOK, encodeFixture(t, fixture))
		}},
		{name: "decode failure", response: func() *http.Response {
			return jsonResponse(http.StatusOK, `{`)
		}},
		{name: "status failure", response: func() *http.Response {
			return jsonResponse(http.StatusNotFound,
				`{"error":{"code":"not_found","message":"m","field":"","runId":""}}`)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := tc.response()
			body, ok := response.Body.(*countingBody)
			require.True(t, ok)
			bodies = append(bodies, body)

			source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
				return response, nil
			})
			_, _ = source.ReceptionSnapshot(t.Context(),
				simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "bravo"}})
			require.Equal(t, 1, body.closed, "the body must be closed exactly once")
		})
	}
}

func TestHTTPSourceRetainsTruncatedReadCauses(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection reset mid body")
	source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          &countingBody{reader: strings.NewReader(""), err: cause},
			ContentLength: 100,
		}, nil
	})

	_, err := source.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, simulatorapi.CategoryUnavailable)
}

func TestHTTPSourceRefusesRedirects(t *testing.T) {
	t.Parallel()

	source, _ := newHTTPSourceFor(t, func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://elsewhere.test/sim/"}},
			Body:       &countingBody{reader: strings.NewReader("")},
			Request:    request,
		}, nil
	})

	_, err := source.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)
	require.ErrorContains(t, err, "redirect")
}

func TestHTTPSourceDoesNotMutateTheHostClient(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: &roundTripper{respond: func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, "{}"), nil
	}}}
	_, err := NewHTTPSource(HTTPSourceConfig{
		BaseURL: sourceBaseURL, Timeout: time.Second,
		MaxRequestBytes: 4096, MaxResponseBytes: simulatorapi.MinResponseBytes, Client: client,
	})
	require.NoError(t, err)
	require.Nil(t, client.CheckRedirect, "the source configures its own copy")
}

func TestHTTPSourcePropagatesCancellationAndDeadlines(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	source, _ := newHTTPSourceFor(t, func(request *http.Request) (*http.Response, error) {
		close(entered)
		select {
		case <-release:
			return jsonResponse(http.StatusOK, "{}"), nil
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := source.ReceptionSnapshot(ctx,
			simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
		done <- err
	}()
	<-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	close(release)

	expired, cancelExpired := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancelExpired()
	deadlineSource, _ := newHTTPSourceFor(t, func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})
	_, err := deadlineSource.ReceptionSnapshot(expired,
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, simulatorapi.CategoryDeadlineExceeded)
}

func TestHTTPSourceBoundsTheEncodedRequest(t *testing.T) {
	t.Parallel()

	transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
		t.Fatal("an oversized request must not be sent")
		return nil, nil
	}}
	source, err := NewHTTPSource(HTTPSourceConfig{
		BaseURL: sourceBaseURL, Timeout: time.Second,
		MaxRequestBytes: 8, MaxResponseBytes: simulatorapi.MinResponseBytes,
		Client: &http.Client{Transport: transport},
	})
	require.NoError(t, err)

	_, err = source.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha", "bravo"}})
	require.ErrorIs(t, err, simulatorapi.CategoryBodyTooLarge)
	require.Empty(t, transport.requests)
}

func TestNewHTTPSourceRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	valid := HTTPSourceConfig{
		BaseURL: sourceBaseURL, Timeout: time.Second,
		MaxRequestBytes: 4096, MaxResponseBytes: simulatorapi.MinResponseBytes,
		Client: &http.Client{},
	}

	cases := []struct {
		name string
		edit func(*HTTPSourceConfig)
	}{
		{name: "relative base", edit: func(c *HTTPSourceConfig) { c.BaseURL = "/sim" }},
		{name: "unsupported scheme", edit: func(c *HTTPSourceConfig) { c.BaseURL = "ftp://host/sim" }},
		{name: "userinfo", edit: func(c *HTTPSourceConfig) { c.BaseURL = "http://u:p@host/sim" }},
		{name: "query", edit: func(c *HTTPSourceConfig) { c.BaseURL = "http://host/sim?a=1" }},
		{name: "fragment", edit: func(c *HTTPSourceConfig) { c.BaseURL = "http://host/sim#a" }},
		{name: "dot segment", edit: func(c *HTTPSourceConfig) { c.BaseURL = "http://host/sim/../x" }},
		{name: "encoded separator", edit: func(c *HTTPSourceConfig) { c.BaseURL = "http://host/a%2Fb" }},
		{name: "zero timeout", edit: func(c *HTTPSourceConfig) { c.Timeout = 0 }},
		{name: "zero request bytes", edit: func(c *HTTPSourceConfig) { c.MaxRequestBytes = 0 }},
		{name: "small response bytes", edit: func(c *HTTPSourceConfig) { c.MaxResponseBytes = 16 }},
		{name: "nil client", edit: func(c *HTTPSourceConfig) { c.Client = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := valid
			tc.edit(&config)
			_, err := NewHTTPSource(config)
			require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
		})
	}
}

func TestHTTPSourcePreservesNestedMountPrefixes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		base string
		want string
	}{
		{base: "http://host", want: "/observations/receptions"},
		{base: "http://host/", want: "/observations/receptions"},
		{base: "http://host/sim", want: "/sim/observations/receptions"},
		{base: "http://host/bench/a/simulator/", want: "/bench/a/simulator/observations/receptions"},
	}
	for _, tc := range cases {
		t.Run(tc.base, func(t *testing.T) {
			t.Parallel()

			transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, `{`), nil
			}}
			source, err := NewHTTPSource(HTTPSourceConfig{
				BaseURL: tc.base, Timeout: time.Second,
				MaxRequestBytes: 4096, MaxResponseBytes: simulatorapi.MinResponseBytes,
				Client: &http.Client{Transport: transport},
			})
			require.NoError(t, err)

			_, _ = source.ReceptionSnapshot(t.Context(),
				simulatorapi.ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
			require.Len(t, transport.requests, 1)
			require.Equal(t, tc.want, transport.requests[0].URL.Path)
		})
	}
}
