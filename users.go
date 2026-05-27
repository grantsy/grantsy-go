package grantsy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grantsy/grantsy-go/internal/api"
)

// UserGetParams contains the parameters for a Users.Get call.
type UserGetParams struct {
	UserID string
	Expand []UserExpand
	// AcceptLanguage sets the Accept-Language request header, controlling the
	// language of localized name/description fields. Empty uses the server default_language.
	AcceptLanguage string
}

// UsersService provides access to user-related API endpoints.
type UsersService struct {
	inner *api.Client
}

// Get returns user information by user ID.
// Optional expand values control which nested fields are populated:
//   - UserExpandPlan: includes the user's plan details
//   - UserExpandFeatures: includes the user's available features
//   - UserExpandSubscription: includes the user's subscription details
func (s *UsersService) Get(ctx context.Context, p UserGetParams) (*Result[*UserResponse], *http.Response, error) {
	params := api.GetV1UsersUserIdParams{AcceptLanguage: optLang(p.AcceptLanguage)}
	if len(p.Expand) > 0 {
		strs := make([]string, len(p.Expand))
		for i, e := range p.Expand {
			strs[i] = string(e)
		}
		params.Expand = &strs
	}
	resp, err := s.inner.GetV1UsersUserId(ctx, p.UserID, &params)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, resp, err
	}
	var result api.GetV1UsersUserID200Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp, err
	}
	return &Result[*UserResponse]{
		Data: userResponseFromAPI(result.Data),
		Meta: result.Meta,
	}, resp, nil
}
