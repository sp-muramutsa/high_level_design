package generator

import (
	"math"
	"testing"
)

func TestBase62EncodeDecode(t *testing.T) {
	testCases := []struct {
		id       uint64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{61, "Z"},
		{62, "10"},
		{125, "21"},
		{10000000, "FXsk"},
		{3521614606207, "ZZZZZZZ"}, // max 7-character Base62 string (62^7 - 1)
		{math.MaxUint64, "lYGhA16ahyf"},
	}

	for _, tc := range testCases {
		encoded := Encode(tc.id)
		if encoded != tc.expected {
			t.Errorf("Encode(%d) = %s; want %s", tc.id, encoded, tc.expected)
		}

		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode(%s) returned unexpected error: %v", encoded, err)
		}
		if decoded != tc.id {
			t.Errorf("Decode(%s) = %d; want %d", encoded, decoded, tc.id)
		}
	}
}

func TestBase62RoundTrip(t *testing.T) {
	sampleIDs := []uint64{0, 1, 42, 999, 1000, 1000000, 999999999, 123456789012345, math.MaxUint64}
	for _, id := range sampleIDs {
		enc := Encode(id)
		dec, err := Decode(enc)
		if err != nil {
			t.Fatalf("Decode error for %d: %v", id, err)
		}
		if dec != id {
			t.Fatalf("Roundtrip failed for %d: got %d", id, dec)
		}
	}
}

func TestBase62DecodeErrors(t *testing.T) {
	_, err := Decode("")
	if err != ErrEmptyString {
		t.Errorf("Decode(\"\") = %v; want ErrEmptyString", err)
	}

	_, err = Decode("abc!123")
	if err != ErrInvalidCharacter {
		t.Errorf("Decode(\"abc!123\") = %v; want ErrInvalidCharacter", err)
	}
}
