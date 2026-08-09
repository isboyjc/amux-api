package common

import "testing"

func TestIsDirectTaskPromptOptional(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "happyhorse-1.1-i2v", want: true},
		{model: "wan2.7-i2v-2026-04-25", want: true},
		{model: "happyhorse-1.1-t2v", want: false},
		{model: "wan2.6-i2v", want: false},
	}

	for _, test := range tests {
		if got := isDirectTaskPromptOptional(test.model); got != test.want {
			t.Errorf("isDirectTaskPromptOptional(%q)=%v, want %v", test.model, got, test.want)
		}
	}
}
