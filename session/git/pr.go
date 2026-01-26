package git

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// PRState represents the state of a pull request
type PRState string

const (
	PRStateNone   PRState = "none"   // No PR exists for the branch
	PRStateOpen   PRState = "open"   // PR is open
	PRStateClosed PRState = "closed" // PR is closed
	PRStateMerged PRState = "merged" // PR is merged
)

// PRInfo holds information about a pull request associated with a branch
type PRInfo struct {
	// Number is the PR number (e.g., 123)
	Number int
	// State is the current state of the PR
	State PRState
	// HasReviewRequired indicates if the PR has a "dev-review-required" label
	HasReviewRequired bool
	// HasAssignee indicates if someone is assigned to the PR
	HasAssignee bool
	// IsApproved indicates if the PR has at least one APPROVED review
	IsApproved bool
	// Error holds any error that occurred during PR info fetch
	Error error
}

// ghPRResponse represents the JSON response from gh pr view
type ghPRResponse struct {
	Number    int       `json:"number"`
	State     string    `json:"state"`
	Labels    []ghLabel `json:"labels"`
	Assignees []ghUser  `json:"assignees"`
	Reviews   ghReviews `json:"reviews"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghUser struct {
	Login string `json:"login"`
}

type ghReviews struct {
	Nodes []ghReviewNode `json:"nodes"`
}

type ghReviewNode struct {
	State string `json:"state"`
}

// DisplayString returns the formatted display string for the PR info
func (p *PRInfo) DisplayString() string {
	if p == nil || p.Error != nil {
		return ""
	}

	switch p.State {
	case PRStateNone:
		return "no PR"
	case PRStateMerged:
		return formatPRNumber(p.Number) + " merged"
	case PRStateClosed:
		return formatPRNumber(p.Number) + " closed"
	case PRStateOpen:
		// Priority: approved > awaiting review > reviewer assigned > open
		if p.HasAssignee && p.IsApproved {
			return formatPRNumber(p.Number) + " approved"
		}
		if p.HasReviewRequired {
			return formatPRNumber(p.Number) + " awaiting review"
		}
		if p.HasAssignee {
			return formatPRNumber(p.Number) + " reviewer assigned"
		}
		return formatPRNumber(p.Number) + " open"
	default:
		return ""
	}
}

func formatPRNumber(n int) string {
	if n == 0 {
		return ""
	}
	return "#" + itoa(n)
}

// itoa converts int to string without importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	if negative {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}

// FetchPRInfo fetches PR information for a given branch using the gh CLI
func FetchPRInfo(repoPath, branchName string) *PRInfo {
	info := &PRInfo{State: PRStateNone}

	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		info.Error = err
		return info
	}

	// Run gh pr view to get PR info for the branch
	cmd := exec.Command("gh", "pr", "view", branchName, "--json", "number,state,labels,assignees,reviews")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		// If the command fails, it likely means no PR exists for this branch
		// This is not an error condition - just means no PR
		return info
	}

	var response ghPRResponse
	if err := json.Unmarshal(output, &response); err != nil {
		info.Error = err
		return info
	}

	info.Number = response.Number

	// Map state
	switch strings.ToUpper(response.State) {
	case "OPEN":
		info.State = PRStateOpen
	case "CLOSED":
		info.State = PRStateClosed
	case "MERGED":
		info.State = PRStateMerged
	default:
		info.State = PRStateNone
	}

	// Check for dev-review-required label (case-insensitive)
	for _, label := range response.Labels {
		if strings.EqualFold(label.Name, "dev-review-required") {
			info.HasReviewRequired = true
			break
		}
	}

	// Check for assignees
	info.HasAssignee = len(response.Assignees) > 0

	// Check for APPROVED reviews
	for _, review := range response.Reviews.Nodes {
		if strings.ToUpper(review.State) == "APPROVED" {
			info.IsApproved = true
			break
		}
	}

	return info
}
