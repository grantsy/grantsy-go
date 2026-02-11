package api

import "fmt"

// Error implements the error interface for ProblemDetails.
func (e *ProblemDetails) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("grantsy: %d %s: %s", e.Status, e.Title, e.Detail)
	}
	return fmt.Sprintf("grantsy: %d %s", e.Status, e.Title)
}
