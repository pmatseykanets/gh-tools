package main

import (
	"testing"
)

func TestLevels(t *testing.T) {
	tests := []struct {
		path   string
		levels int
	}{
		{"a", 1},
		{"a/b", 2},
		{"a/b/c", 3},
		{"", 1},
		{"/", 2},
		{"a//b", 3},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if want, got := tt.levels, levels(tt.path); want != got {
				t.Errorf("Expected levels %d got %d", want, got)
			}
		})
	}
}
