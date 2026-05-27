package grantsy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grantsy/grantsy-go/internal/api"
)

// FeatureListParams contains the parameters for a Features.List call.
type FeatureListParams struct {
	// AcceptLanguage sets the Accept-Language request header, controlling the
	// language of localized name/description fields. Empty uses the server default_language.
	AcceptLanguage string
}

// FeatureGetParams contains the parameters for a Features.Get call.
type FeatureGetParams struct {
	FeatureID string
	// AcceptLanguage sets the Accept-Language request header, controlling the
	// language of localized name/description fields. Empty uses the server default_language.
	AcceptLanguage string
}

// FeaturesService provides access to feature-related API endpoints.
type FeaturesService struct {
	inner *api.Client
}

// List returns all available features.
func (s *FeaturesService) List(ctx context.Context, p FeatureListParams) (*Result[[]Feature], *http.Response, error) {
	params := api.GetV1FeaturesParams{AcceptLanguage: optLang(p.AcceptLanguage)}
	resp, err := s.inner.GetV1Features(ctx, &params)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, resp, err
	}
	var result api.GetV1Features200Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp, err
	}
	var features []Feature
	if result.Data != nil {
		features = result.Data.Features
	}
	return &Result[[]Feature]{
		Data: features,
		Meta: result.Meta,
	}, resp, nil
}

// Get returns a single feature by ID.
func (s *FeaturesService) Get(ctx context.Context, p FeatureGetParams) (*Result[*Feature], *http.Response, error) {
	params := api.GetV1FeaturesFeatureIdParams{AcceptLanguage: optLang(p.AcceptLanguage)}
	resp, err := s.inner.GetV1FeaturesFeatureId(ctx, p.FeatureID, &params)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, resp, err
	}
	var result api.GetV1FeaturesFeatureID200Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp, err
	}
	var feature *Feature
	if result.Data != nil {
		feature = &result.Data.Feature
	}
	return &Result[*Feature]{
		Data: feature,
		Meta: result.Meta,
	}, resp, nil
}
