package api

import "fmt"

// Error implements the error interface for ProblemDetails.
func (e *ProblemDetails) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("grantsy: %d %s: %s", e.Status, e.Title, e.Detail)
	}
	return fmt.Sprintf("grantsy: %d %s", e.Status, e.Title)
}

// StatusCode returns the HTTP status code.
func (e *ProblemDetails) StatusCode() int {
	return e.Status
}

// ErrorType returns the error type string.
func (e *ProblemDetails) ErrorType() ProblemDetailsType {
	return ProblemDetailsType(e.Type)
}
