package main

import (
	"testing"
)

func TestParseNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string returns default",
			input:    "",
			expected: []string{"default"},
		},
		{
			name:     "single namespace",
			input:    "kube-system",
			expected: []string{"kube-system"},
		},
		{
			name:     "multiple namespaces",
			input:    "kube-system,monitoring,default",
			expected: []string{"kube-system", "monitoring", "default"},
		},
		{
			name:     "namespaces with spaces",
			input:    "kube-system, monitoring , default",
			expected: []string{"kube-system", "monitoring", "default"},
		},
		{
			name:     "duplicate namespaces",
			input:    "default,default,monitoring",
			expected: []string{"default", "default", "monitoring"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNamespaces(tt.input)
			
			if len(result) != len(tt.expected) {
				t.Errorf("parseNamespaces() returned %d namespaces, expected %d", len(result), len(tt.expected))
			}
			
			for i, ns := range result {
				if i < len(tt.expected) && ns != tt.expected[i] {
					t.Errorf("parseNamespaces()[%d] = %q, expected %q", i, ns, tt.expected[i])
				}
			}
		})
	}
}

func TestParseTTL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty string returns default",
			input:    "",
			expected: 300,
		},
		{
			name:     "valid integer",
			input:    "600",
			expected: 600,
		},
		{
			name:     "zero value",
			input:    "0",
			expected: 0,
		},
		{
			name:     "negative value",
			input:    "-100",
			expected: -100,
		},
		{
			name:     "invalid string returns default",
			input:    "not-a-number",
			expected: 300,
		},
		{
			name:     "very large number",
			input:    "86400",
			expected: 86400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output to avoid test noise
			result := parseTTL(tt.input)
			
			if result != tt.expected {
				t.Errorf("parseTTL(%q) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}