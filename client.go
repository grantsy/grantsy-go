package grantsy

import (
	"context"
	"encoding/json"
	"io"
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
	ErrBadRequest       ErrorType = ErrorType(api.HTTPSGrantsyExampleErrorsBadRequest)
	ErrNotFound         ErrorType = ErrorType(api.HTTPSGrantsyExampleErrorsNotFound)
	ErrUnauthorized     ErrorType = ErrorType(api.HTTPSGrantsyExampleErrorsUnauthorized)
	ErrValidationFailed ErrorType = ErrorType(api.HTTPSGrantsyExampleErrorsValidationFailed)
	ErrInternalError    ErrorType = ErrorType(api.HTTPSGrantsyExampleErrorsInternalError)
)

// FieldError represents a validation error on a specific field.
type FieldError = api.FieldError

// CheckResponse is the result of a feature access check.
type CheckResponse struct {
	// Whether the user has access to this feature
	Allowed bool `json:"allowed"`
	// The checked feature (requires expand=feature)
	Feature *Feature `json:"feature,omitempty"`
	// The user's current plan (requires expand=plan or expand=plan.features)
	Plan *Plan `json:"plan,omitempty"`
	// Reason for the access decision
	Reason CheckResponseReason `json:"reason"`
	// The user ID
	UserID string `json:"user_id"`
}

// CheckResponseReason is the reason for a check access decision.
type CheckResponseReason = api.CheckResponseReason

// Check access reasons.
const (
	ReasonDefaultPlan      = api.DefaultPlan
	ReasonFeatureInPlan    = api.FeatureInPlan
	ReasonInsufficientPlan = api.InsufficientPlan
	ReasonNoSubscription   = api.NoSubscription
)

// CheckParams contains the parameters for a Check call.
type CheckParams struct {
	UserID  string
	Feature string
	Expand  []CheckExpand
	// AcceptLanguage sets the Accept-Language request header, controlling the
	// language of localized name/description fields. Empty uses the server default_language.
	AcceptLanguage string
}

// CheckExpand controls which fields are expanded in a Check response.
type CheckExpand = api.CheckExpand

// Check expand constants.
const (
	CheckExpandFeature      = api.CheckExpandFeature
	CheckExpandPlan         = api.CheckExpandPlan
	CheckExpandPlanFeatures = api.CheckExpandPlanFeatures
)

// Feature represents a feature definition.
type Feature = api.Feature

// Plan represents a subscription plan.
type Plan = api.Plan

// Variant represents a pricing variant for a plan.
type Variant = api.Variant

// PlanExpand controls which fields are expanded in a Plans.Get response.
type PlanExpand = api.PlanExpand

// Plan expand constant for single-plan get.
const PlanExpandFeatures = api.PlanExpandFeatures

// PlansExpand controls which fields are expanded in a Plans.List response.
type PlansExpand = api.PlansExpand

// Plans expand constant for list.
const PlansExpandFeatures = api.PlansExpandFeatures

// UserResponse is the result of getting user info.
type UserResponse struct {
	// Features available to the user (requires expand=features)
	Features []Feature `json:"features,omitempty"`
	// Plan details (requires expand=plan)
	Plan *Plan `json:"plan,omitempty"`
	// The user's current plan ID
	PlanID string `json:"plan_id"`
	// Subscription details (requires expand=subscription)
	Subscription *UserSubscription `json:"subscription,omitempty"`
	// The user ID
	UserID string `json:"user_id"`
}

// UserSubscription contains subscription summary fields.
type UserSubscription = api.UserSubscription

// UserExpand controls which fields are expanded in a Users.Get response.
type UserExpand = api.UserExpand

// User expand constants.
const (
	UserExpandPlan         = api.UserExpandPlan
	UserExpandFeatures     = api.UserExpandFeatures
	UserExpandSubscription = api.UserExpandSubscription
)

// Meta contains response metadata from the API.
type Meta = api.Meta

// Result is the generic envelope for all API responses.
// Data contains the parsed response payload, Meta contains request metadata.
type Result[T any] struct {
	Data T
	Meta *Meta
}

// Nullable is a generic type that can distinguish between unspecified, null, and valued fields.
type Nullable[T any] = api.Nullable[T]

// RawSubscription contains the raw provider-specific subscription data.
type RawSubscription = api.RawSubscription

// RawSubscriptionProvider identifies the subscription provider.
type RawSubscriptionProvider = api.RawSubscriptionProvider

// ProviderSubscription is a union type holding provider-specific subscription data.
type ProviderSubscription = api.ProviderSubscription

// LemonSqueezySubscription contains LemonSqueezy-specific subscription fields.
type LemonSqueezySubscription = api.LemonSqueezySubscription

// Subscription provider constants.
const ProviderLemonSqueezy = api.Lemonsqueezy

// ClientOption allows setting custom parameters during construction.
type ClientOption = api.ClientOption

// WithHTTPClient allows overriding the default HTTP client.
var WithHTTPClient = api.WithHTTPClient

// Client is the Grantsy API client.
type Client struct {
	inner *api.Client

	// Features provides access to feature-related endpoints.
	Features *FeaturesService
	// Plans provides access to plan-related endpoints.
	Plans *PlansService
	// Users provides access to user-related endpoints.
	Users *UsersService
}

// New creates a new Grantsy API client with API key authentication.
func New(baseURL, apiKey string, opts ...ClientOption) (*Client, error) {
	opts = append(opts, api.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("X-Api-Key", apiKey)
		return nil
	}))
	inner, err := api.NewClient(baseURL, opts...)
	if err != nil {
		return nil, err
	}
	c := &Client{inner: inner}
	c.Features = &FeaturesService{inner: inner}
	c.Plans = &PlansService{inner: inner}
	c.Users = &UsersService{inner: inner}
	return c, nil
}

// Check checks if a user has access to a feature.
func (c *Client) Check(ctx context.Context, p CheckParams) (*Result[*CheckResponse], *http.Response, error) {
	params := api.GetV1CheckParams{
		UserId:         &p.UserID,
		Feature:        &p.Feature,
		AcceptLanguage: optLang(p.AcceptLanguage),
	}
	if len(p.Expand) > 0 {
		expandStrs := make([]string, len(p.Expand))
		for i, e := range p.Expand {
			expandStrs[i] = string(e)
		}
		params.Expand = &expandStrs
	}
	resp, err := c.inner.GetV1Check(ctx, &params)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, resp, err
	}
	var result api.GetV1Check200Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp, err
	}
	return &Result[*CheckResponse]{
		Data: checkResponseFromAPI(result.Data),
		Meta: result.Meta,
	}, resp, nil
}

// optLang returns a pointer to lang for the Accept-Language header parameter,
// or nil when lang is empty so no header is sent and the server uses its
// configured default_language. The value is sent verbatim, so it may be a
// single BCP-47 tag ("es", "en-US") or a full list ("es,en;q=0.8").
func optLang(lang string) *string {
	if lang == "" {
		return nil
	}
	return &lang
}

// checkResponse inspects the HTTP response and returns an error if the
// status code is not 200.
func checkResponse(resp *http.Response) error {
	if resp.StatusCode == 200 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &APIError{Status: resp.StatusCode, Title: http.StatusText(resp.StatusCode)}
	}
	var errResp api.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return &APIError{Status: resp.StatusCode, Title: http.StatusText(resp.StatusCode)}
	}
	errResp.Error.Status = resp.StatusCode
	return &errResp.Error
}

func checkResponseFromAPI(r *api.CheckResponse) *CheckResponse {
	if r == nil {
		return nil
	}
	cr := &CheckResponse{
		Allowed: r.Allowed,
		Reason:  r.Reason,
		UserID:  r.UserID,
	}
	if r.Feature != nil {
		cr.Feature = r.Feature.Feature
	}
	if r.Plan != nil {
		cr.Plan = r.Plan.Plan
	}
	return cr
}

func userResponseFromAPI(r *api.UserResponse) *UserResponse {
	if r == nil {
		return nil
	}
	ur := &UserResponse{
		Features: r.Features,
		PlanID:   r.PlanID,
		UserID:   r.UserID,
	}
	if r.Plan != nil {
		ur.Plan = r.Plan.Plan
	}
	if r.Subscription != nil {
		ur.Subscription = r.Subscription.UserSubscription
	}
	return ur
}
