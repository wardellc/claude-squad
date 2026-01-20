package git

import (
	"testing"
)

func TestPRInfoDisplayString(t *testing.T) {
	tests := []struct {
		name     string
		prInfo   *PRInfo
		expected string
	}{
		{
			name:     "nil PRInfo",
			prInfo:   nil,
			expected: "",
		},
		{
			name:     "no PR",
			prInfo:   &PRInfo{State: PRStateNone},
			expected: "no PR",
		},
		{
			name:     "open PR",
			prInfo:   &PRInfo{Number: 123, State: PRStateOpen},
			expected: "#123 open",
		},
		{
			name:     "open PR awaiting review",
			prInfo:   &PRInfo{Number: 456, State: PRStateOpen, HasReviewRequired: true},
			expected: "#456 awaiting review",
		},
		{
			name:     "open PR reviewer assigned",
			prInfo:   &PRInfo{Number: 789, State: PRStateOpen, HasAssignee: true},
			expected: "#789 reviewer assigned",
		},
		{
			name:     "open PR awaiting review takes precedence over reviewer assigned",
			prInfo:   &PRInfo{Number: 100, State: PRStateOpen, HasReviewRequired: true, HasAssignee: true},
			expected: "#100 awaiting review",
		},
		{
			name:     "merged PR",
			prInfo:   &PRInfo{Number: 50, State: PRStateMerged},
			expected: "#50 merged",
		},
		{
			name:     "closed PR",
			prInfo:   &PRInfo{Number: 25, State: PRStateClosed},
			expected: "#25 closed",
		},
		{
			name:     "PR with error returns empty",
			prInfo:   &PRInfo{Number: 10, State: PRStateOpen, Error: &testError{}},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.prInfo == nil {
				result = ""
			} else {
				result = tt.prInfo.DisplayString()
			}
			if result != tt.expected {
				t.Errorf("DisplayString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// testError implements error interface for testing
type testError struct{}

func (e *testError) Error() string {
	return "test error"
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{9999, "9999"},
	}

	for _, tt := range tests {
		result := itoa(tt.input)
		if result != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatPRNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, ""},
		{1, "#1"},
		{123, "#123"},
		{9999, "#9999"},
	}

	for _, tt := range tests {
		result := formatPRNumber(tt.input)
		if result != tt.expected {
			t.Errorf("formatPRNumber(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
