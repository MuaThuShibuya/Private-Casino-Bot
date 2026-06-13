package ui

import "testing"

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{10000, "10.000"},
		{0, "0"},
		{-10000, "-10.000"},
		{-15000, "-15.000"},
		{1000000, "1.000.000"},
		{-1000000, "-1.000.000"},
	}

	for _, tc := range tests {
		actual := FormatNumber(tc.input)
		if actual != tc.expected {
			t.Errorf("FormatNumber(%d) = %s, mong muốn %s", tc.input, actual, tc.expected)
		}
	}
}
