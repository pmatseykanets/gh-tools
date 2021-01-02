package size

import (
	"fmt"
	"testing"
)

func TestSizeParse(t *testing.T) {
	tests := []struct {
		input string
		value int64
		err   error
	}{
		{"", 0, nil},
		{"0", 0, nil},
		{"1", 1, nil},
		{"b", 1, nil},
		{"10b", 10, nil},
		{"10b", 10, nil},
		{"500mb", 500 * MByte, nil},
		{"24Gb", 24 * GByte, nil},
		{"18 Tib", 18 * TiByte, nil},
		{"5 EiB", 5 * EiByte, nil},
		{"foo", 0, errSyntax},
		{"5bar", 0, errSyntax},
		{"10 KBaz", 0, errSyntax},
		{"-5 Mb", 0, errSyntax},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			value, err := Parse(tt.input)
			if want, got := tt.err, err; want != got {
				t.Fatalf("Expected error %s got %s", want, got)
			}
			if want, got := tt.value, value; want != got {
				t.Errorf("Expected value %d got %d", want, got)
			}
		})
	}
}

func TestSizeFormatBytes(t *testing.T) {
	tests := []struct {
		value  int64
		result string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{KByte, "1.0 kB"},
		{2*KByte - 1, "2.0 kB"},
		{28 * TByte, "28.0 TB"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprint(tt.value), func(t *testing.T) {
			t.Parallel()

			if want, got := tt.result, FormatBytes(tt.value); want != got {
				t.Fatalf("Expected %s got %s", want, got)
			}
		})
	}
}

func TestSizeFormatIBytes(t *testing.T) {
	tests := []struct {
		value  int64
		result string
	}{
		{0, "0 B"},
		{KiByte - 1, "1023 B"},
		{KiByte, "1.0 KiB"},
		{2*KiByte - 1, "2.0 KiB"},
		{28 * TiByte, "28.0 TiB"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprint(tt.value), func(t *testing.T) {
			t.Parallel()

			if want, got := tt.result, FormatIBytes(tt.value); want != got {
				t.Fatalf("Expected %s got %s", want, got)
			}
		})
	}
}
