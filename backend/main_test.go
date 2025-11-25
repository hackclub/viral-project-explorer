package main

import (
	"database/sql"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    sql.NullString
		expected interface{}
	}{
		{
			name:     "lowercase GitHub URL",
			input:    sql.NullString{String: "https://GitHub.com", Valid: true},
			expected: "https://github.com",
		},
		{
			name:     "add https prefix",
			input:    sql.NullString{String: "github.com", Valid: true},
			expected: "https://github.com",
		},
		{
			name:     "trim whitespace and lowercase",
			input:    sql.NullString{String: "  GITHUB.COM  ", Valid: true},
			expected: "https://github.com",
		},
		{
			name:     "preserve http prefix",
			input:    sql.NullString{String: "http://Example.com", Valid: true},
			expected: "http://example.com",
		},
		{
			name:     "handle extra spaces in URL",
			input:    sql.NullString{String: "github .com/user/repo", Valid: true},
			expected: "https://github.com/user/repo",
		},
		{
			name:     "full URL with path",
			input:    sql.NullString{String: "https://GitHub.com/HackClub/Sprig", Valid: true},
			expected: "https://github.com/hackclub/sprig",
		},
		{
			name:     "null string returns nil",
			input:    sql.NullString{String: "", Valid: false},
			expected: nil,
		},
		{
			name:     "empty string returns nil",
			input:    sql.NullString{String: "", Valid: true},
			expected: nil,
		},
		{
			name:     "whitespace only returns nil",
			input:    sql.NullString{String: "   ", Valid: true},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %v, want %v", tt.input.String, result, tt.expected)
			}
		})
	}
}

