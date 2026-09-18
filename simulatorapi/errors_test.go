package simulatorapi_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

func TestAPIErrorMatchesCategoryAfterWrapping(t *testing.T) {
	t.Parallel()

	cause := errors.New("engine refused the station")
	err := simulatorapi.WrapError(simulatorapi.CategoryConflict, cause, "station revision is stale").
		WithField("$.expectedRevision").WithRunID("run-7")

	wrapped := fmt.Errorf("update station: %w", err)
	require.ErrorIs(t, wrapped, simulatorapi.CategoryConflict)
	require.NotErrorIs(t, wrapped, simulatorapi.CategoryInvalid)
	require.ErrorIs(t, wrapped, cause)

	var typed *simulatorapi.APIError
	require.ErrorAs(t, wrapped, &typed)
	require.Equal(t, "run-7", typed.RunID)
	require.Equal(t, "$.expectedRevision", typed.Field)
	require.Contains(t, err.Error(), "conflict: station revision is stale")
}

func TestAPIErrorWireShapeOmitsCause(t *testing.T) {
	t.Parallel()

	err := simulatorapi.WrapError(simulatorapi.CategoryInvalid, errors.New("secret detail"), "count is out of range").
		WithField("$.count")

	data, marshalErr := json.Marshal(simulatorapi.ErrorResponse{Error: *err})
	require.NoError(t, marshalErr)
	require.JSONEq(t,
		`{"error":{"code":"invalid","message":"count is out of range","field":"$.count","runId":""}}`,
		string(data))
	require.NotContains(t, string(data), "secret detail")

	var round simulatorapi.ErrorResponse
	require.NoError(t, json.Unmarshal(data, &round))
	require.ErrorIs(t, &round.Error, simulatorapi.CategoryInvalid)
	require.NoError(t, round.Error.Unwrap())
}

func TestNewErrorClampsExternalText(t *testing.T) {
	t.Parallel()

	err := simulatorapi.NewError(simulatorapi.CategoryInternal, strings.Repeat("x", simulatorapi.MaxMessageBytes*2))
	require.Len(t, err.Message, simulatorapi.MaxMessageBytes)
}

func TestCategoryValidRejectsUnknownWireCodes(t *testing.T) {
	t.Parallel()

	for _, category := range []simulatorapi.Category{
		simulatorapi.CategoryInvalid, simulatorapi.CategoryNotFound,
		simulatorapi.CategoryMethodNotAllowed, simulatorapi.CategoryConflict,
		simulatorapi.CategoryBodyTooLarge, simulatorapi.CategoryUnsupportedMediaType,
		simulatorapi.CategoryLimit, simulatorapi.CategoryUnavailable,
		simulatorapi.CategoryResponseLimit, simulatorapi.CategoryDeadlineExceeded,
		simulatorapi.CategorySourceInvalid, simulatorapi.CategoryInternal,
	} {
		require.True(t, category.Valid(), category)
	}
	require.False(t, simulatorapi.Category("").Valid())
	require.False(t, simulatorapi.Category("teapot").Valid())
}

func TestWithFieldAndRunIDDoNotMutateTheOriginal(t *testing.T) {
	t.Parallel()

	base := simulatorapi.NewError(simulatorapi.CategoryLimit, "too many stations")
	derived := base.WithField("$.station.id").WithRunID("run-1")

	require.Empty(t, base.Field)
	require.Empty(t, base.RunID)
	require.Equal(t, "$.station.id", derived.Field)
	require.Equal(t, "run-1", derived.RunID)
}
