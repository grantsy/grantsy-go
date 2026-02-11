package grantsy

import (
	"context"
	"net/http"

	"github.com/grantsy/grantsy-go/internal/api"
)

// APIError is returned when the API responds with a non-200 status code.
// Use errors.As to access the structured error details.
type APIError = api.ProblemDetails

// ErrorType represents the type of API error.
type ErrorType = api.ProblemDetailsType

// API error types.
const (
	ErrBadRequest       = api.HTTPSGrantsyExampleErrorsBadRequest
	ErrUnauthorized     = api.HTTPSGrantsyExampleErrorsUnauthorized
	ErrValidationFailed = api.HTTPSGrantsyExampleErrorsValidationFailed
	ErrInternalError    = api.HTTPSGrantsyExampleErrorsInternalError
)

// FieldError represents a validation error on a specific field.
type FieldError = api.FieldError

// CheckAccessParams are the parameters for CheckAccess.
type CheckAccessParams = api.GetV1CheckParams

// CheckResult is the result of a feature access check.
type CheckResult = api.CheckResult

// ListFeaturesParams are the parameters for ListFeatures.
type ListFeaturesParams = api.GetV1FeaturesParams

// UserFeatures is the result of listing a user's features.
type UserFeatures = api.FeaturesResponse

// ListPlansParams are the parameters for ListPlans.
type ListPlansParams = api.GetV1PlansParams

// Plan represents a subscription plan.
type Plan = api.PlanDTO

// Feature represents a feature definition.
type Feature = api.FeatureDTO

// Variant represents a pricing variant for a plan.
type Variant = api.VariantDTO

// PlansResult is the result of listing plans.
type PlansResult = api.PlansResponse

// GetSubscriptionParams are the parameters for GetSubscription.
type GetSubscriptionParams = api.GetV1SubscriptionParams

// Subscription is the result of getting a user's subscription.
type Subscription = api.SubscriptionResponse

// ClientOption allows setting custom parameters during construction.
type ClientOption = api.ClientOption

// WithHTTPClient allows overriding the default HTTP client.
var WithHTTPClient = api.WithHTTPClient

// Client is the Grantsy API client.
type Client struct {
	inner *api.ClientWithResponses
}

// New creates a new Grantsy API client with API key authentication.
func New(baseURL, apiKey string, opts ...ClientOption) (*Client, error) {
	opts = append(opts, api.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("X-Api-Key", apiKey)
		return nil
	}))
	cwr, err := api.NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{inner: cwr}, nil
}

// CheckAccess checks if a user has access to a feature.
func (c *Client) CheckAccess(ctx context.Context, params CheckAccessParams) (*CheckResult, error) {
	resp, err := c.inner.GetV1CheckWithResponse(ctx, &params)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.StatusCode(), resp.JSON400, resp.JSON401, resp.JSON422, resp.JSON500); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, &APIError{Status: resp.StatusCode(), Title: "unexpected response"}
	}
	return &resp.JSON200.Data, nil
}

// ListFeatures lists features available to a user.
func (c *Client) ListFeatures(ctx context.Context, params ListFeaturesParams) (*UserFeatures, error) {
	resp, err := c.inner.GetV1FeaturesWithResponse(ctx, &params)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.StatusCode(), resp.JSON400, resp.JSON401, resp.JSON422, resp.JSON500); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, &APIError{Status: resp.StatusCode(), Title: "unexpected response"}
	}
	return &resp.JSON200.Data, nil
}

// ListPlans lists available plans.
func (c *Client) ListPlans(ctx context.Context, params ListPlansParams) (*PlansResult, error) {
	resp, err := c.inner.GetV1PlansWithResponse(ctx, &params)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.StatusCode(), resp.JSON400, resp.JSON401, resp.JSON422, resp.JSON500); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, &APIError{Status: resp.StatusCode(), Title: "unexpected response"}
	}
	return &resp.JSON200.Data, nil
}

// GetSubscription gets a user's subscription.
func (c *Client) GetSubscription(ctx context.Context, params GetSubscriptionParams) (*Subscription, error) {
	resp, err := c.inner.GetV1SubscriptionWithResponse(ctx, &params)
	if err != nil {
		return nil, err
	}
	if err := checkResponse(resp.StatusCode(), resp.JSON400, resp.JSON401, resp.JSON422, resp.JSON500); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, &APIError{Status: resp.StatusCode(), Title: "unexpected response"}
	}
	return &resp.JSON200.Data, nil
}

func checkResponse(statusCode int, json400, json401, json422, json500 *api.ErrorResponse) error {
	var errResp *api.ErrorResponse
	switch {
	case statusCode == 200:
		return nil
	case json400 != nil:
		errResp = json400
	case json401 != nil:
		errResp = json401
	case json422 != nil:
		errResp = json422
	case json500 != nil:
		errResp = json500
	default:
		return &APIError{Status: statusCode, Title: http.StatusText(statusCode)}
	}

	errResp.Error.Status = statusCode
	return &errResp.Error
}
