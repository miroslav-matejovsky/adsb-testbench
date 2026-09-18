package simulatorapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// ErrBodyTooLarge reports that an input exceeded its explicit byte bound. It
// is separate from ordinary syntax failures so transports can map it to their
// own status.
var ErrBodyTooLarge = errors.New("input exceeds the configured byte limit")

// Value is one JSON value from a strict parse. The tree records presence, so
// callers distinguish an omitted key, an explicit null, and a zero value that
// ordinary Go scalars cannot express.
//
// A Value is read through typed accessors. Every accessor reports the JSON
// path of the offending value, so parsers need no separate path bookkeeping.
type Value struct {
	path    string
	kind    valueKind
	text    string
	number  json.Number
	boolean bool
	fields  map[string]*Value
	order   []string
	items   []*Value
}

type valueKind uint8

const (
	kindNull valueKind = iota
	kindBool
	kindNumber
	kindString
	kindObject
	kindArray
)

func (k valueKind) String() string {
	switch k {
	case kindNull:
		return "null"
	case kindBool:
		return "boolean"
	case kindNumber:
		return "number"
	case kindString:
		return "string"
	case kindObject:
		return "object"
	default:
		return "array"
	}
}

// ParseJSON reads at most maxBytes from r and returns one strictly parsed
// JSON value. Duplicate object keys, trailing values, and oversized input are
// rejected. maxBytes must be positive.
func ParseJSON(r io.Reader, maxBytes int) (*Value, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("maxBytes %d must be positive", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(r, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read JSON input: %w", err)
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("%w: input exceeds %d bytes", ErrBodyTooLarge, maxBytes)
	}
	return ParseJSONBytes(data)
}

// ParseJSONBytes strictly parses one complete JSON value from data.
func ParseJSONBytes(data []byte) (*Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := parseValue(decoder, "$")
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON input has a trailing value after the first one")
	}
	return value, nil
}

// parseValue reads exactly one value and its descendants.
func parseValue(decoder *json.Decoder, path string) (*Value, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read JSON at %s: %w", path, err)
	}
	return parseFromToken(decoder, path, token)
}

// parseFromToken completes one value whose first token was already read.
func parseFromToken(decoder *json.Decoder, path string, token json.Token) (*Value, error) {
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case objectStart:
			return parseObject(decoder, path)
		case arrayStart:
			return parseArray(decoder, path)
		default:
			return nil, fmt.Errorf("unexpected %q at %s", typed, path)
		}
	case nil:
		return &Value{path: path, kind: kindNull}, nil
	case bool:
		return &Value{path: path, kind: kindBool, boolean: typed}, nil
	case json.Number:
		return &Value{path: path, kind: kindNumber, number: typed}, nil
	case string:
		return &Value{path: path, kind: kindString, text: typed}, nil
	default:
		return nil, fmt.Errorf("unsupported JSON token at %s", path)
	}
}

// JSON structural delimiters used by the token walk.
const (
	objectStart = json.Delim('{')
	objectEnd   = json.Delim('}')
	arrayStart  = json.Delim('[')
	arrayEnd    = json.Delim(']')
)

func parseObject(decoder *json.Decoder, path string) (*Value, error) {
	value := &Value{path: path, kind: kindObject, fields: map[string]*Value{}}
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read JSON object at %s: %w", path, err)
		}
		if delim, ok := token.(json.Delim); ok && delim == objectEnd {
			return value, nil
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key at %s", path)
		}
		if _, exists := value.fields[key]; exists {
			return nil, fmt.Errorf("duplicate object key %q at %s", key, path)
		}
		child, err := parseValue(decoder, path+"."+key)
		if err != nil {
			return nil, err
		}
		value.fields[key] = child
		value.order = append(value.order, key)
	}
}

func parseArray(decoder *json.Decoder, path string) (*Value, error) {
	value := &Value{path: path, kind: kindArray, items: []*Value{}}
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read JSON array at %s: %w", path, err)
		}
		if delim, ok := token.(json.Delim); ok && delim == arrayEnd {
			return value, nil
		}
		child, err := parseFromToken(decoder, fmt.Sprintf("%s[%d]", path, len(value.items)), token)
		if err != nil {
			return nil, err
		}
		value.items = append(value.items, child)
	}
}

// Path returns the JSON path of the value, for error reporting.
func (v *Value) Path() string { return v.path }

// IsNull reports whether the value is an explicit JSON null.
func (v *Value) IsNull() bool { return v.kind == kindNull }

// Object asserts an object value and returns a presence-tracking reader.
func (v *Value) Object() (*Object, error) {
	if v.kind != kindObject {
		return nil, v.wrongKind(kindObject)
	}
	return &Object{value: v, used: map[string]bool{}}, nil
}

// Array asserts an array value and returns its elements in order.
func (v *Value) Array() ([]*Value, error) {
	if v.kind != kindArray {
		return nil, v.wrongKind(kindArray)
	}
	return v.items, nil
}

// Text asserts a string value.
func (v *Value) Text() (string, error) {
	if v.kind != kindString {
		return "", v.wrongKind(kindString)
	}
	return v.text, nil
}

// Bool asserts a boolean value.
func (v *Value) Bool() (bool, error) {
	if v.kind != kindBool {
		return false, v.wrongKind(kindBool)
	}
	return v.boolean, nil
}

// Float asserts a finite JSON number.
func (v *Value) Float() (float64, error) {
	if v.kind != kindNumber {
		return 0, v.wrongKind(kindNumber)
	}
	number, err := v.number.Float64()
	if err != nil {
		return 0, fmt.Errorf("read number at %s: %w", v.path, err)
	}
	if !Finite(number) {
		return 0, fmt.Errorf("number at %s must be finite", v.path)
	}
	return number, nil
}

// Int asserts a JSON integer within the platform int range.
func (v *Value) Int() (int, error) {
	if v.kind != kindNumber {
		return 0, v.wrongKind(kindNumber)
	}
	value, err := strconv.ParseInt(v.number.String(), 10, 0)
	if err != nil {
		return 0, fmt.Errorf("read integer at %s: %w", v.path, err)
	}
	return int(value), nil
}

// String implements fmt.Stringer with the value's path and kind. Use Text for
// the contents of a JSON string.
func (v *Value) String() string { return v.path + " (" + v.kind.String() + ")" }

func (v *Value) wrongKind(want valueKind) error {
	return fmt.Errorf("value at %s is %s, want %s", v.path, v.kind, want)
}

// Object reads the fields of one JSON object and tracks which keys a parser
// consumed, so unknown keys are rejected exactly once at the end.
type Object struct {
	value *Value
	used  map[string]bool
}

// Path returns the JSON path of the object.
func (o *Object) Path() string { return o.value.path }

// Field returns a required, non-null member.
func (o *Object) Field(name string) (*Value, error) {
	child, err := o.member(name)
	if err != nil {
		return nil, err
	}
	if child.IsNull() {
		return nil, fmt.Errorf("required field %s is null", child.path)
	}
	return child, nil
}

// Nullable returns a required member that may be an explicit null, reported
// as a nil value. An omitted key is still an error.
func (o *Object) Nullable(name string) (*Value, error) {
	child, err := o.member(name)
	if err != nil {
		return nil, err
	}
	if child.IsNull() {
		return nil, nil
	}
	return child, nil
}

// Done rejects every key the parser did not consume.
func (o *Object) Done() error {
	for _, key := range o.value.order {
		if !o.used[key] {
			return fmt.Errorf("unknown field %s.%s", o.value.path, key)
		}
	}
	return nil
}

func (o *Object) member(name string) (*Value, error) {
	o.used[name] = true
	child, ok := o.value.fields[name]
	if !ok {
		return nil, fmt.Errorf("missing required field %s.%s", o.value.path, name)
	}
	return child, nil
}
