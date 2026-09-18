package simulatorapi

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Representable instants. Every wire timestamp stays inside the four-digit
// year range so formatted output is unambiguous.
const (
	minYear = 1
	maxYear = 9999
)

// FrameBytes is the exact length of one encoded ADS-B frame.
const FrameBytes = 14

// ICAO address domain of the codec, shared by every wire representation.
const (
	minICAO = 0x000001
	maxICAO = 0xFFFFFE
)

// MaxMessageBytes bounds externally derived error text. Longer text is
// clamped so an error envelope always fits the minimum response budget.
const MaxMessageBytes = 512

// MinResponseBytes is the smallest response budget that always holds a
// complete error envelope with a clamped message, a run identifier, and a
// field path. Configurations below this bound are rejected.
const MinResponseBytes = 2048

// FormatUint64 returns the canonical decimal representation of v.
func FormatUint64(v uint64) string { return strconv.FormatUint(v, 10) }

// ParseUint64 accepts the canonical decimal representation of an unsigned
// 64-bit value: "0", or digits without a leading zero. Signs, whitespace,
// and other bases are rejected.
func ParseUint64(text string) (uint64, error) {
	if err := canonicalDigits(text); err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse unsigned value %q: %w", text, err)
	}
	return value, nil
}

// FormatDuration returns the canonical decimal nanosecond representation of d.
func FormatDuration(d time.Duration) string { return strconv.FormatInt(int64(d), 10) }

// ParseDuration accepts a canonical decimal nanosecond string within signed
// time.Duration bounds. "-0" and leading zeros are rejected.
func ParseDuration(text string) (time.Duration, error) {
	digits := text
	if strings.HasPrefix(digits, "-") {
		digits = digits[1:]
		if digits == "0" {
			return 0, fmt.Errorf("duration %q is not canonical: negative zero", text)
		}
	}
	if err := canonicalDigits(digits); err != nil {
		return 0, fmt.Errorf("duration %q: %w", text, err)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q nanoseconds: %w", text, err)
	}
	return time.Duration(value), nil
}

// FormatTime returns t as a UTC RFC3339Nano string.
func FormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// ParseTime accepts a UTC RFC3339Nano string within years 1-9999. Offsets
// other than "Z" are rejected so one instant has one representation.
func ParseTime(text string) (time.Time, error) {
	if !strings.HasSuffix(text, "Z") {
		return time.Time{}, fmt.Errorf("time %q must be UTC and end with Z", text)
	}
	value, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", text, err)
	}
	value = value.UTC()
	if year := value.Year(); year < minYear || year > maxYear {
		return time.Time{}, fmt.Errorf("time %q year %d is outside %d-%d", text, year, minYear, maxYear)
	}
	return value, nil
}

// FormatICAO returns the six uppercase hexadecimal digits of an address.
func FormatICAO(icao uint32) string { return fmt.Sprintf("%06X", icao) }

// ParseICAO accepts six uppercase hexadecimal digits within 000001..FFFFFE.
func ParseICAO(text string) (uint32, error) {
	if len(text) != 6 {
		return 0, fmt.Errorf("ICAO %q has %d characters, want 6", text, len(text))
	}
	value, err := parseUpperHex(text)
	if err != nil {
		return 0, fmt.Errorf("parse ICAO %q: %w", text, err)
	}
	if value < minICAO || value > maxICAO {
		return 0, fmt.Errorf("ICAO %q is outside %06X-%06X", text, minICAO, maxICAO)
	}
	return uint32(value), nil
}

// FormatFrame returns the 28 uppercase hexadecimal digits of one frame.
func FormatFrame(frame [FrameBytes]byte) string {
	const hexDigits = "0123456789ABCDEF"
	out := make([]byte, 0, 2*FrameBytes)
	for _, b := range frame {
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0F])
	}
	return string(out)
}

// ParseFrame accepts exactly 28 uppercase hexadecimal digits.
func ParseFrame(text string) ([FrameBytes]byte, error) {
	var frame [FrameBytes]byte
	if len(text) != 2*FrameBytes {
		return frame, fmt.Errorf("frame %q has %d characters, want %d", text, len(text), 2*FrameBytes)
	}
	for i := range FrameBytes {
		high, err := hexNibble(text[2*i])
		if err != nil {
			return [FrameBytes]byte{}, fmt.Errorf("parse frame %q: %w", text, err)
		}
		low, err := hexNibble(text[2*i+1])
		if err != nil {
			return [FrameBytes]byte{}, fmt.Errorf("parse frame %q: %w", text, err)
		}
		frame[i] = high<<4 | low
	}
	return frame, nil
}

// ClampMessage shortens externally derived text to MaxMessageBytes so it
// always fits the minimum response budget. Truncation is marked.
func ClampMessage(text string) string {
	if len(text) <= MaxMessageBytes {
		return text
	}
	const ellipsis = "..."
	return text[:MaxMessageBytes-len(ellipsis)] + ellipsis
}

// Finite reports whether v is an ordinary floating-point number. Non-finite
// values have no JSON representation and are rejected on every path.
func Finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// canonicalDigits checks "0" or digits without a leading zero.
func canonicalDigits(text string) error {
	if text == "" {
		return fmt.Errorf("value %q is empty, want decimal digits", text)
	}
	for i := range len(text) {
		if text[i] < '0' || text[i] > '9' {
			return fmt.Errorf("value %q must contain only decimal digits", text)
		}
	}
	if len(text) > 1 && text[0] == '0' {
		return fmt.Errorf("value %q is not canonical: leading zero", text)
	}
	return nil
}

// parseUpperHex reads uppercase hexadecimal digits into a 64-bit value.
func parseUpperHex(text string) (uint64, error) {
	var value uint64
	for i := range len(text) {
		nibble, err := hexNibble(text[i])
		if err != nil {
			return 0, err
		}
		value = value<<4 | uint64(nibble)
	}
	return value, nil
}

// hexNibble converts one uppercase hexadecimal digit.
func hexNibble(b byte) (byte, error) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', nil
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("byte %q is not an uppercase hexadecimal digit", b)
	}
}
