package simulatorapi_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

func TestParseJSONRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "duplicate key", body: `{"a":1,"a":2}`},
		{name: "duplicate nested key", body: `{"a":{"b":1,"b":2}}`},
		{name: "trailing value", body: `{"a":1} {"a":2}`},
		{name: "trailing token", body: `{"a":1}]`},
		{name: "truncated object", body: `{"a":1`},
		{name: "empty", body: ``},
		{name: "not json", body: `a`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := simulatorapi.ParseJSON(strings.NewReader(tc.body), 4096)
			require.Error(t, err)
		})
	}
}

func TestParseJSONBoundsInputBytes(t *testing.T) {
	t.Parallel()

	body := `{"a":1}`
	value, err := simulatorapi.ParseJSON(strings.NewReader(body), len(body))
	require.NoError(t, err)
	require.NotNil(t, value)

	_, err = simulatorapi.ParseJSON(strings.NewReader(body), len(body)-1)
	require.ErrorIs(t, err, simulatorapi.ErrBodyTooLarge)

	_, err = simulatorapi.ParseJSON(strings.NewReader(body), 0)
	require.Error(t, err)
}

func TestObjectDistinguishesMissingNullAndZero(t *testing.T) {
	t.Parallel()

	value, err := simulatorapi.ParseJSONBytes([]byte(`{"count":0,"enabled":false,"ids":[],"cursor":null}`))
	require.NoError(t, err)
	object, err := value.Object()
	require.NoError(t, err)

	count, err := object.Field("count")
	require.NoError(t, err)
	number, err := count.Int()
	require.NoError(t, err)
	require.Equal(t, 0, number)

	enabled, err := object.Field("enabled")
	require.NoError(t, err)
	flag, err := enabled.Bool()
	require.NoError(t, err)
	require.False(t, flag)

	ids, err := object.Field("ids")
	require.NoError(t, err)
	items, err := ids.Array()
	require.NoError(t, err)
	require.Empty(t, items)

	cursor, err := object.Nullable("cursor")
	require.NoError(t, err)
	require.Nil(t, cursor)

	_, err = object.Field("cursor")
	require.ErrorContains(t, err, "is null")

	_, err = object.Field("missing")
	require.ErrorContains(t, err, "missing required field $.missing")

	require.NoError(t, object.Done())
}

func TestObjectDoneRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	value, err := simulatorapi.ParseJSONBytes([]byte(`{"known":1,"extra":2}`))
	require.NoError(t, err)
	object, err := value.Object()
	require.NoError(t, err)
	_, err = object.Field("known")
	require.NoError(t, err)
	require.ErrorContains(t, object.Done(), "unknown field $.extra")
}

func TestValueAccessorsRejectWrongKinds(t *testing.T) {
	t.Parallel()

	value, err := simulatorapi.ParseJSONBytes([]byte(`{"text":"a","number":1e400,"big":99999999999999999999}`))
	require.NoError(t, err)
	object, err := value.Object()
	require.NoError(t, err)

	text, err := object.Field("text")
	require.NoError(t, err)
	_, err = text.Int()
	require.ErrorContains(t, err, "want number")
	_, err = text.Bool()
	require.Error(t, err)
	_, err = text.Array()
	require.Error(t, err)
	_, err = text.Object()
	require.Error(t, err)

	overflow, err := object.Field("number")
	require.NoError(t, err)
	_, err = overflow.Float()
	require.Error(t, err)

	big, err := object.Field("big")
	require.NoError(t, err)
	_, err = big.Int()
	require.Error(t, err)

	require.NoError(t, object.Done())
}

func TestValuePathsIdentifyNestedFailures(t *testing.T) {
	t.Parallel()

	value, err := simulatorapi.ParseJSONBytes([]byte(`{"spawn":{"latitudeDegrees":{"min":1}}}`))
	require.NoError(t, err)
	object, err := value.Object()
	require.NoError(t, err)
	spawn, err := object.Field("spawn")
	require.NoError(t, err)
	spawnObject, err := spawn.Object()
	require.NoError(t, err)
	latitude, err := spawnObject.Field("latitudeDegrees")
	require.NoError(t, err)
	require.Equal(t, "$.spawn.latitudeDegrees", latitude.Path())
	latitudeObject, err := latitude.Object()
	require.NoError(t, err)
	_, err = latitudeObject.Field("max")
	require.ErrorContains(t, err, "$.spawn.latitudeDegrees.max")
}

func TestParseJSONReportsReadFailures(t *testing.T) {
	t.Parallel()

	want := errors.New("read failed")
	_, err := simulatorapi.ParseJSON(failingReader{err: want}, 16)
	require.ErrorIs(t, err, want)
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }
