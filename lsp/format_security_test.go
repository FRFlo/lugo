package lsp

import "testing"

func TestValidateFormattingOptionsTabSize(t *testing.T) {
	tests := []struct {
		name    string
		tabSize int
		valid   bool
	}{
		{name: "minimum", tabSize: 1, valid: true},
		{name: "maximum", tabSize: 16, valid: true},
		{name: "negative", tabSize: -1},
		{name: "zero", tabSize: 0},
		{name: "too large", tabSize: 17},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFormattingOptions(FormattingOptions{TabSize: tt.tabSize})
			if (err == nil) != tt.valid {
				t.Fatalf("validateFormattingOptions(%d) error = %v, valid = %v", tt.tabSize, err, tt.valid)
			}
		})
	}
}
