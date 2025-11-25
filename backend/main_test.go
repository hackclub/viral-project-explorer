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
			input:    sql.NullString{String: "https://GitHub.com/SomeOrg/SomeRepo", Valid: true},
			expected: "https://github.com/someorg/somerepo",
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
		{
			name:     "remove .git suffix from GitHub URL",
			input:    sql.NullString{String: "https://github.com/user/my-project.git", Valid: true},
			expected: "https://github.com/user/my-project",
		},
		{
			name:     "remove .git suffix without scheme",
			input:    sql.NullString{String: "github.com/user/repo.git", Valid: true},
			expected: "https://github.com/user/repo",
		},
		{
			name:     "remove /tree/branch from GitHub URL",
			input:    sql.NullString{String: "https://github.com/user/my-project/tree/master", Valid: true},
			expected: "https://github.com/user/my-project/",
		},
		{
			name:     "remove /tree/branch with nested path from GitHub URL",
			input:    sql.NullString{String: "https://github.com/user/repo/tree/main/src/components", Valid: true},
			expected: "https://github.com/user/repo/",
		},
		{
			name:     "preserve /blob/ path in GitHub URL",
			input:    sql.NullString{String: "https://github.com/user/repo/blob/main/src/file.txt", Valid: true},
			expected: "https://github.com/user/repo/blob/main/src/file.txt",
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


