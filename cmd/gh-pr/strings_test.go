package main

import (
	"fmt"
	"testing"
)

func TestContains(t *testing.T) {
	tests := []struct {
		hay    []string
		needle string
		found  bool
	}{
		{[]string{"foo"}, "foo", true},
		{[]string{"bar", "foo"}, "foo", true},
		{[]string{"Foo"}, "foo", true},
		{nil, "foo", false},
		{[]string{}, "foo", false},
		{[]string{"bar"}, "foo", false},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if want, got := tt.found, contains(tt.hay, tt.needle); want != got {
				t.Errorf("Expected %v got %v", want, got)
			}
		})
	}
}
