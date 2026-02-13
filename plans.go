package grantsy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grantsy/grantsy-go/internal/api"
)

// PlanListParams contains the parameters for a Plans.List call.
type PlanListParams struct {
	Expand []PlansExpand
}

// PlanGetParams contains the parameters for a Plans.Get call.
type PlanGetParams struct {
	PlanID string
	Expand []PlanExpand
}

// PlansService provides access to plan-related API endpoints.
type PlansService struct {
	inner *api.Client
}

// List returns all available plans.
// Optional expand values control which nested fields are populated
// (e.g., PlansExpandFeatures to include each plan's features).
func (s *PlansService) List(ctx context.Context, p PlanListParams) (*Result[[]Plan], *http.Response, error) {
	params := api.GetV1PlansParams{}
	if len(p.Expand) > 0 {
		strs := make([]string, len(p.Expand))
		for i, e := range p.Expand {
			strs[i] = string(e)
		}
		params.Expand = &strs
	}
	resp, err := s.inner.GetV1Plans(ctx, &params)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, resp, err
	}
	var result api.GetV1Plans200Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp, err
	}
	var plans []Plan
	if result.Data != nil {
		plans = result.Data.Plans
	}
	return &Result[[]Plan]{
		Data: plans,
		Meta: result.Meta,
	}, resp, nil
}

// Get returns a single plan by ID.
// Optional expand values control which nested fields are populated
// (e.g., PlanExpandFeatures to include the plan's features).
func (s *PlansService) Get(ctx context.Context, p PlanGetParams) (*Result[*Plan], *http.Response, error) {
	params := api.GetV1PlansPlanIdParams{}
	if len(p.Expand) > 0 {
		strs := make([]string, len(p.Expand))
		for i, e := range p.Expand {
			strs[i] = string(e)
		}
		params.Expand = &strs
	}
	resp, err := s.inner.GetV1PlansPlanId(ctx, p.PlanID, &params)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, resp, err
	}
	var result api.GetV1PlansPlanID200Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp, err
	}
	var plan *Plan
	if result.Data != nil {
		plan = &result.Data.Plan
	}
	return &Result[*Plan]{
		Data: plan,
		Meta: result.Meta,
	}, resp, nil
}
