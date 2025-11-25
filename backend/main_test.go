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
			expected: "https://github.com/user/my-project",
		},
		{
			name:     "remove /tree/branch with nested path from GitHub URL",
			input:    sql.NullString{String: "https://github.com/user/repo/tree/main/src/components", Valid: true},
			expected: "https://github.com/user/repo",
		},
		// Trailing slash normalization tests
		{
			name:     "remove trailing slash from GitHub repo URL",
			input:    sql.NullString{String: "https://github.com/someuser/somerepo/", Valid: true},
			expected: "https://github.com/someuser/somerepo",
		},
		{
			name:     "URL without trailing slash stays the same",
			input:    sql.NullString{String: "https://github.com/someuser/somerepo", Valid: true},
			expected: "https://github.com/someuser/somerepo",
		},
		{
			name:     "remove multiple trailing slashes",
			input:    sql.NullString{String: "https://github.com/someuser/somerepo///", Valid: true},
			expected: "https://github.com/someuser/somerepo",
		},
		{
			name:     "trailing slash with .git suffix",
			input:    sql.NullString{String: "https://github.com/someuser/somerepo.git/", Valid: true},
			expected: "https://github.com/someuser/somerepo",
		},
		{
			name:     "preserve /blob/ path in GitHub URL",
			input:    sql.NullString{String: "https://github.com/user/repo/blob/main/src/file.txt", Valid: true},
			expected: "https://github.com/user/repo/blob/main/src/file.txt",
		},
		// Security: Dangerous URL scheme tests
		{
			name:     "reject javascript: scheme",
			input:    sql.NullString{String: "javascript:alert(1)", Valid: true},
			expected: nil,
		},
		{
			name:     "reject JavaScript: scheme (mixed case)",
			input:    sql.NullString{String: "JavaScript:alert(document.cookie)", Valid: true},
			expected: nil,
		},
		{
			name:     "reject JAVASCRIPT: scheme (uppercase)",
			input:    sql.NullString{String: "JAVASCRIPT:void(0)", Valid: true},
			expected: nil,
		},
		{
			name:     "reject data: scheme",
			input:    sql.NullString{String: "data:text/html,<script>alert(1)</script>", Valid: true},
			expected: nil,
		},
		{
			name:     "reject DATA: scheme (uppercase)",
			input:    sql.NullString{String: "DATA:text/html,<script>alert(1)</script>", Valid: true},
			expected: nil,
		},
		{
			name:     "reject vbscript: scheme",
			input:    sql.NullString{String: "vbscript:msgbox(1)", Valid: true},
			expected: nil,
		},
		{
			name:     "reject file: scheme",
			input:    sql.NullString{String: "file:///etc/passwd", Valid: true},
			expected: nil,
		},
		{
			name:     "reject javascript: with spaces (after normalization)",
			input:    sql.NullString{String: "java script:alert(1)", Valid: true},
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


