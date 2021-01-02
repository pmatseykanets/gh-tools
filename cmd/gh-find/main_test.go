package main

import (
	"fmt"
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

func TestSizePredicateMatch(t *testing.T) {
	tests := []struct {
		op    int
		value int64
		size  int64
		is    bool
	}{
		{-1, 1024, 1023, true},
		{-1, 1024, 1024, true},
		{-1, 1023, 1024, false},
		{0, 1024, 1024, true},
		{0, 1024, 1023, false},
		{0, 1024, 1025, false},
		{1, 1024, 1024, true},
		{1, 1024, 1025, true},
		{1, 1024, 1023, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprint(tt.op, tt.value, tt.size), func(t *testing.T) {
			t.Parallel()
			p := &sizePredicate{op: tt.op, value: tt.value}
			if want, got := tt.is, p.match(tt.size); want != got {
				t.Errorf("Expected %v got %v", want, got)
			}
		})
	}
}
