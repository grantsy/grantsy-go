package grantsy_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	_ "modernc.org/sqlite"

	grantsy "github.com/grantsy/grantsy-go"
)

const testAPIKey = "test-api-key"

const testConfig = `env: dev
server:
  host: 0.0.0.0
  port: 8080
database:
  driver: sqlite
  dsn: /data/grantsy.db
entitlements:
  default_plan: free
  default_language: en
  plans:
    - id: free
      name: { en: Free }
      features: [dashboard]
    - id: pro
      name: { en: Pro }
      features: [dashboard, api, sso]
  features:
    - id: dashboard
      name: { en: Dashboard }
      description: { en: Basic dashboard access }
    - id: api
      name: { en: API Access }
      description: { en: REST API access }
    - id: sso
      name: { en: Single Sign-On }
      description: { en: SSO integration }
auth:
  api_key: test-api-key
providers:
  lemonsqueezy:
    api_key: dummy-api-key
    products:
      - product_id: 12345
        plan_id: pro
    webhook:
      secret: dummy-secret
log:
  level: debug
  format: text
`

func seedDB(t *testing.T, dbPath string) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS subscriptions_lemonsqueezy (
		id                   INTEGER PRIMARY KEY,
		user_id              TEXT NOT NULL UNIQUE,
		customer_id          INTEGER NOT NULL DEFAULT 0,
		order_id             INTEGER NOT NULL DEFAULT 0,
		product_id           INTEGER NOT NULL DEFAULT 0,
		product_name         TEXT NOT NULL DEFAULT '',
		variant_id           INTEGER NOT NULL DEFAULT 0,
		variant_name         TEXT NOT NULL DEFAULT '',
		status               TEXT NOT NULL DEFAULT '',
		status_formatted     TEXT NOT NULL DEFAULT '',
		card_brand           TEXT NOT NULL DEFAULT '',
		card_last_four       TEXT NOT NULL DEFAULT '',
		cancelled            BOOLEAN NOT NULL DEFAULT FALSE,
		trial_ends_at        INTEGER,
		billing_anchor       INTEGER NOT NULL DEFAULT 0,
		subscription_item_id INTEGER NOT NULL DEFAULT 0,
		price_id             INTEGER NOT NULL DEFAULT 0,
		unit_price           INTEGER NOT NULL DEFAULT 0,
		renewal_interval_unit TEXT NOT NULL DEFAULT '',
		renewal_interval_quantity INTEGER NOT NULL DEFAULT 0,
		renews_at            INTEGER NOT NULL DEFAULT 0,
		ends_at              INTEGER,
		created_at           INTEGER NOT NULL DEFAULT 0,
		updated_at           INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO subscriptions_lemonsqueezy
		(id, user_id, product_id, product_name, status, status_formatted, renews_at, created_at, updated_at)
		VALUES (1, 'user-pro', 12345, 'Pro', 'active', 'Active', ?, ?, ?)`,
		time.Now().Add(30*24*time.Hour).Unix(),
		time.Now().Unix(),
		time.Now().Unix(),
	)
	require.NoError(t, err)
}

func startGrantsy(t *testing.T) (*grantsy.Client, string) {
	t.Helper()
	ctx := context.Background()

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(testConfig), 0644))

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "grantsy.db")
	seedDB(t, dbPath)

	// Create a Docker volume and seed it with the DB file via a busybox container.
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	require.NoError(t, err)

	vol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { dockerClient.VolumeRemove(ctx, vol.Name, true) })

	seedCtr, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "busybox:latest",
				Cmd:   []string{"sleep", "infinity"},
				HostConfigModifier: func(hc *container.HostConfig) {
					hc.Mounts = append(hc.Mounts, mount.Mount{
						Type:   mount.TypeVolume,
						Source: vol.Name,
						Target: "/data",
					})
				},
			},
			Started: true,
		},
	)
	require.NoError(t, err)
	require.NoError(
		t,
		seedCtr.CopyFileToContainer(ctx, dbPath, "/data/grantsy.db", 0666),
	)
	_, _, err = seedCtr.Exec(ctx, []string{"chown", "-R", "65534:65534", "/data"})
	require.NoError(t, err)
	require.NoError(t, seedCtr.Terminate(ctx))

	ctr, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:           "ghcr.io/grantsy/grantsy:main",
				AlwaysPullImage: true,
				ExposedPorts:    []string{"8080/tcp"},
				HostConfigModifier: func(hc *container.HostConfig) {
					hc.Mounts = append(hc.Mounts, mount.Mount{
						Type:   mount.TypeVolume,
						Source: vol.Name,
						Target: "/data",
					})
				},
				Files: []testcontainers.ContainerFile{
					{
						HostFilePath:      configPath,
						ContainerFilePath: "/etc/grantsy/config.yaml",
						FileMode:          0644,
					},
				},
				WaitingFor: wait.ForHTTP("/healthz").
					WithPort("8080/tcp").
					WithStartupTimeout(30 * time.Second),
			},
			Started: true,
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() { ctr.Terminate(ctx) })

	host, err := ctr.Host(ctx)
	require.NoError(t, err)
	port, err := ctr.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	baseURL := fmt.Sprintf("http://%s:%s", host, port.Port())
	grantsyClient, err := grantsy.New(baseURL, testAPIKey)
	require.NoError(t, err)

	return grantsyClient, baseURL
}

func strPtr(s string) *string { return &s }

func assertFeature(
	t *testing.T,
	f grantsy.Feature,
	wantID, wantName string,
	wantDesc *string,
) {
	t.Helper()
	assert.Equal(t, wantID, f.ID)
	assert.Equal(t, wantName, f.Name)
	if wantDesc != nil {
		require.NotNil(t, f.Description)
		assert.Equal(t, *wantDesc, *f.Description)
	} else {
		assert.Nil(t, f.Description)
	}
}

func assertFeatureIDs(
	t *testing.T,
	features []grantsy.Feature,
	wantIDs []string,
) {
	t.Helper()
	ids := make([]string, len(features))
	for i, f := range features {
		ids[i] = f.ID
	}
	assert.ElementsMatch(t, wantIDs, ids)
}

func assertProSubscription(t *testing.T, sub *grantsy.UserSubscription) {
	t.Helper()

	assert.Equal(t, "active", sub.Status)
	assert.Equal(t, "pro", sub.PlanID)
	assert.False(t, sub.Cancelled)
	assert.True(t, sub.EndsAt.IsNull())
	assert.False(t, sub.RenewsAt.IsNull())
	renewsAt, err := sub.RenewsAt.Get()
	require.NoError(t, err)
	assert.Greater(t, renewsAt, 0)
	assert.True(t, sub.TrialEndsAt.IsNull())

	assert.Equal(t, grantsy.ProviderLemonSqueezy, sub.Raw.Provider)
	require.NotNil(t, sub.Raw.Data.LemonSqueezySubscription)
	ls := sub.Raw.Data.LemonSqueezySubscription

	assert.Equal(t, 1, ls.ID)
	assert.Equal(t, 0, ls.CustomerID)
	assert.Equal(t, 0, ls.OrderID)
	assert.Equal(t, 12345, ls.ProductID)
	assert.Equal(t, "Pro", ls.ProductName)
	assert.Equal(t, 0, ls.VariantID)
	assert.Equal(t, "", ls.VariantName)
	assert.Equal(t, "active", ls.Status)
	assert.Equal(t, "Active", ls.StatusFormatted)
	assert.Equal(t, "", ls.CardBrand)
	assert.Equal(t, "", ls.CardLastFour)
	assert.False(t, ls.Cancelled)
	assert.Equal(t, 0, ls.BillingAnchor)
	assert.Equal(t, 0, ls.SubscriptionItemID)
	assert.Equal(t, 0, ls.PriceID)
	assert.Equal(t, 0, ls.UnitPrice)
	assert.Equal(t, "", ls.RenewalIntervalUnit)
	assert.Equal(t, 0, ls.RenewalIntervalQuantity)
	assert.True(t, ls.EndsAt.IsNull())
	assert.True(t, ls.TrialEndsAt.IsNull())
	assert.Greater(t, ls.RenewsAt, int64(0))
	assert.Greater(t, ls.CreatedAt, int64(0))
	assert.Greater(t, ls.UpdatedAt, int64(0))
}

func featureIDs(features []grantsy.Feature) []string {
	ids := make([]string, len(features))
	for i, f := range features {
		ids[i] = f.ID
	}
	return ids
}

func TestIntegration(t *testing.T) {
	c, baseURL := startGrantsy(t)
	ctx := context.Background()

	t.Run("Check", func(t *testing.T) {
		tests := []struct {
			name string
			give grantsy.CheckParams
			want grantsy.CheckResponse
		}{
			{
				name: "free user/dashboard/no expand",
				give: grantsy.CheckParams{UserID: "user-1", Feature: "dashboard"},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonDefaultPlan,
					UserID:  "user-1",
				},
			},
			{
				name: "free user/api/no expand",
				give: grantsy.CheckParams{UserID: "user-1", Feature: "api"},
				want: grantsy.CheckResponse{
					Allowed: false,
					Reason:  grantsy.ReasonInsufficientPlan,
					UserID:  "user-1",
				},
			},
			{
				name: "free user/dashboard/expand feature",
				give: grantsy.CheckParams{
					UserID:  "user-1",
					Feature: "dashboard",
					Expand:  []grantsy.CheckExpand{grantsy.CheckExpandFeature},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonDefaultPlan,
					UserID:  "user-1",
					Feature: &grantsy.Feature{
						ID:          "dashboard",
						Name:        "Dashboard",
						Description: strPtr("Basic dashboard access"),
					},
				},
			},
			{
				name: "free user/dashboard/expand plan",
				give: grantsy.CheckParams{
					UserID:  "user-1",
					Feature: "dashboard",
					Expand:  []grantsy.CheckExpand{grantsy.CheckExpandPlan},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonDefaultPlan,
					UserID:  "user-1",
					Plan:    &grantsy.Plan{ID: "free", Name: "Free"},
				},
			},
			{
				name: "free user/dashboard/expand feature+plan",
				give: grantsy.CheckParams{
					UserID:  "user-1",
					Feature: "dashboard",
					Expand: []grantsy.CheckExpand{
						grantsy.CheckExpandFeature,
						grantsy.CheckExpandPlan,
					},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonDefaultPlan,
					UserID:  "user-1",
					Feature: &grantsy.Feature{
						ID:          "dashboard",
						Name:        "Dashboard",
						Description: strPtr("Basic dashboard access"),
					},
					Plan: &grantsy.Plan{ID: "free", Name: "Free"},
				},
			},
			{
				name: "free user/dashboard/expand plan.features",
				give: grantsy.CheckParams{
					UserID:  "user-1",
					Feature: "dashboard",
					Expand:  []grantsy.CheckExpand{grantsy.CheckExpandPlanFeatures},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonDefaultPlan,
					UserID:  "user-1",
					Plan: &grantsy.Plan{
						ID:   "free",
						Name: "Free",
						Features: []grantsy.Feature{
							{
								ID:          "dashboard",
								Name:        "Dashboard",
								Description: strPtr("Basic dashboard access"),
							},
						},
					},
				},
			},
			{
				name: "free user/dashboard/expand all",
				give: grantsy.CheckParams{
					UserID:  "user-1",
					Feature: "dashboard",
					Expand: []grantsy.CheckExpand{
						grantsy.CheckExpandFeature,
						grantsy.CheckExpandPlan,
						grantsy.CheckExpandPlanFeatures,
					},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonDefaultPlan,
					UserID:  "user-1",
					Feature: &grantsy.Feature{
						ID:          "dashboard",
						Name:        "Dashboard",
						Description: strPtr("Basic dashboard access"),
					},
					Plan: &grantsy.Plan{
						ID:   "free",
						Name: "Free",
						Features: []grantsy.Feature{
							{
								ID:          "dashboard",
								Name:        "Dashboard",
								Description: strPtr("Basic dashboard access"),
							},
						},
					},
				},
			},
			{
				name: "free user/api/expand feature+plan",
				give: grantsy.CheckParams{
					UserID:  "user-1",
					Feature: "api",
					Expand: []grantsy.CheckExpand{
						grantsy.CheckExpandFeature,
						grantsy.CheckExpandPlan,
					},
				},
				want: grantsy.CheckResponse{
					Allowed: false,
					Reason:  grantsy.ReasonInsufficientPlan,
					UserID:  "user-1",
					Feature: &grantsy.Feature{
						ID:          "api",
						Name:        "API Access",
						Description: strPtr("REST API access"),
					},
					Plan: &grantsy.Plan{ID: "free", Name: "Free"},
				},
			},
			{
				name: "pro user/api/no expand",
				give: grantsy.CheckParams{UserID: "user-pro", Feature: "api"},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonFeatureInPlan,
					UserID:  "user-pro",
				},
			},
			{
				name: "pro user/api/expand feature+plan",
				give: grantsy.CheckParams{
					UserID:  "user-pro",
					Feature: "api",
					Expand: []grantsy.CheckExpand{
						grantsy.CheckExpandFeature,
						grantsy.CheckExpandPlan,
					},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonFeatureInPlan,
					UserID:  "user-pro",
					Feature: &grantsy.Feature{
						ID:          "api",
						Name:        "API Access",
						Description: strPtr("REST API access"),
					},
					Plan: &grantsy.Plan{ID: "pro", Name: "Pro"},
				},
			},
			{
				name: "pro user/api/expand plan.features",
				give: grantsy.CheckParams{
					UserID:  "user-pro",
					Feature: "api",
					Expand:  []grantsy.CheckExpand{grantsy.CheckExpandPlanFeatures},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonFeatureInPlan,
					UserID:  "user-pro",
					Plan: &grantsy.Plan{
						ID:   "pro",
						Name: "Pro",
						Features: []grantsy.Feature{
							{
								ID:          "dashboard",
								Name:        "Dashboard",
								Description: strPtr("Basic dashboard access"),
							},
							{
								ID:          "api",
								Name:        "API Access",
								Description: strPtr("REST API access"),
							},
							{
								ID:          "sso",
								Name:        "Single Sign-On",
								Description: strPtr("SSO integration"),
							},
						},
					},
				},
			},
			{
				name: "pro user/sso/expand all",
				give: grantsy.CheckParams{
					UserID:  "user-pro",
					Feature: "sso",
					Expand: []grantsy.CheckExpand{
						grantsy.CheckExpandFeature,
						grantsy.CheckExpandPlan,
						grantsy.CheckExpandPlanFeatures,
					},
				},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonFeatureInPlan,
					UserID:  "user-pro",
					Feature: &grantsy.Feature{
						ID:          "sso",
						Name:        "Single Sign-On",
						Description: strPtr("SSO integration"),
					},
					Plan: &grantsy.Plan{
						ID:   "pro",
						Name: "Pro",
						Features: []grantsy.Feature{
							{
								ID:          "dashboard",
								Name:        "Dashboard",
								Description: strPtr("Basic dashboard access"),
							},
							{
								ID:          "api",
								Name:        "API Access",
								Description: strPtr("REST API access"),
							},
							{
								ID:          "sso",
								Name:        "Single Sign-On",
								Description: strPtr("SSO integration"),
							},
						},
					},
				},
			},
			{
				name: "no-sub user/dashboard/no expand",
				give: grantsy.CheckParams{UserID: "user-no-sub", Feature: "dashboard"},
				want: grantsy.CheckResponse{
					Allowed: true,
					Reason:  grantsy.ReasonDefaultPlan,
					UserID:  "user-no-sub",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, _, err := c.Check(ctx, tt.give)
				require.NoError(t, err)

				assert.Equal(t, tt.want.Allowed, result.Data.Allowed)
				assert.Equal(t, tt.want.UserID, result.Data.UserID)
				assert.Equal(t, tt.want.Reason, result.Data.Reason)

				if tt.want.Feature == nil {
					assert.Nil(t, result.Data.Feature)
				} else {
					require.NotNil(t, result.Data.Feature)
					assertFeature(t, *result.Data.Feature, tt.want.Feature.ID, tt.want.Feature.Name, tt.want.Feature.Description)
				}

				if tt.want.Plan == nil {
					assert.Nil(t, result.Data.Plan)
				} else {
					require.NotNil(t, result.Data.Plan)
					assert.Equal(t, tt.want.Plan.ID, result.Data.Plan.ID)
					assert.Equal(t, tt.want.Plan.Name, result.Data.Plan.Name)
					if tt.want.Plan.Features == nil {
						assert.Empty(t, result.Data.Plan.Features)
					} else {
						assertFeatureIDs(t, result.Data.Plan.Features, featureIDs(tt.want.Plan.Features))
					}
				}
			})
		}
	})

	t.Run("Features.List", func(t *testing.T) {
		result, _, err := c.Features.List(ctx, grantsy.FeatureListParams{})
		require.NoError(t, err)
		require.Len(t, result.Data, 3)

		fm := make(map[string]grantsy.Feature, len(result.Data))
		for _, f := range result.Data {
			fm[f.ID] = f
		}

		assertFeature(
			t,
			fm["dashboard"],
			"dashboard",
			"Dashboard",
			strPtr("Basic dashboard access"),
		)
		assertFeature(t, fm["api"], "api", "API Access", strPtr("REST API access"))
		assertFeature(
			t,
			fm["sso"],
			"sso",
			"Single Sign-On",
			strPtr("SSO integration"),
		)
	})

	t.Run("Features.Get", func(t *testing.T) {
		tests := []struct {
			name string
			give grantsy.FeatureGetParams
			want grantsy.Feature
		}{
			{
				name: "dashboard",
				give: grantsy.FeatureGetParams{FeatureID: "dashboard"},
				want: grantsy.Feature{
					ID:          "dashboard",
					Name:        "Dashboard",
					Description: strPtr("Basic dashboard access"),
				},
			},
			{
				name: "api",
				give: grantsy.FeatureGetParams{FeatureID: "api"},
				want: grantsy.Feature{
					ID:          "api",
					Name:        "API Access",
					Description: strPtr("REST API access"),
				},
			},
			{
				name: "sso",
				give: grantsy.FeatureGetParams{FeatureID: "sso"},
				want: grantsy.Feature{
					ID:          "sso",
					Name:        "Single Sign-On",
					Description: strPtr("SSO integration"),
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, _, err := c.Features.Get(ctx, tt.give)
				require.NoError(t, err)
				assertFeature(
					t,
					*result.Data,
					tt.want.ID,
					tt.want.Name,
					tt.want.Description,
				)
			})
		}
	})

	t.Run("Plans.List", func(t *testing.T) {
		tests := []struct {
			name string
			give grantsy.PlanListParams
			want []grantsy.Plan
		}{
			{
				name: "no expand",
				give: grantsy.PlanListParams{},
				want: []grantsy.Plan{
					{ID: "free", Name: "Free"},
					{ID: "pro", Name: "Pro"},
				},
			},
			{
				name: "expand features",
				give: grantsy.PlanListParams{
					Expand: []grantsy.PlansExpand{grantsy.PlansExpandFeatures},
				},
				want: []grantsy.Plan{
					{
						ID:   "free",
						Name: "Free",
						Features: []grantsy.Feature{
							{ID: "dashboard"},
						},
					},
					{
						ID:   "pro",
						Name: "Pro",
						Features: []grantsy.Feature{
							{ID: "dashboard"},
							{ID: "api"},
							{ID: "sso"},
						},
					},
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, _, err := c.Plans.List(ctx, tt.give)
				require.NoError(t, err)
				require.Len(t, result.Data, len(tt.want))

				pm := make(map[string]grantsy.Plan, len(result.Data))
				for _, p := range result.Data {
					pm[p.ID] = p
				}
				for _, wp := range tt.want {
					p, ok := pm[wp.ID]
					require.True(t, ok, "plan %s not found", wp.ID)
					assert.Equal(t, wp.Name, p.Name)
					assert.Nil(t, p.Description)
					assert.Empty(t, p.Variants)
					if wp.Features == nil {
						assert.Empty(t, p.Features)
					} else {
						assertFeatureIDs(t, p.Features, featureIDs(wp.Features))
					}
				}
			})
		}
	})

	t.Run("Plans.Get", func(t *testing.T) {
		tests := []struct {
			name string
			give grantsy.PlanGetParams
			want grantsy.Plan
		}{
			{
				name: "free/no expand",
				give: grantsy.PlanGetParams{PlanID: "free"},
				want: grantsy.Plan{ID: "free", Name: "Free"},
			},
			{
				name: "free/expand features",
				give: grantsy.PlanGetParams{
					PlanID: "free",
					Expand: []grantsy.PlanExpand{grantsy.PlanExpandFeatures},
				},
				want: grantsy.Plan{
					ID:   "free",
					Name: "Free",
					Features: []grantsy.Feature{
						{ID: "dashboard"},
					},
				},
			},
			{
				name: "pro/no expand",
				give: grantsy.PlanGetParams{PlanID: "pro"},
				want: grantsy.Plan{ID: "pro", Name: "Pro"},
			},
			{
				name: "pro/expand features",
				give: grantsy.PlanGetParams{
					PlanID: "pro",
					Expand: []grantsy.PlanExpand{grantsy.PlanExpandFeatures},
				},
				want: grantsy.Plan{
					ID:   "pro",
					Name: "Pro",
					Features: []grantsy.Feature{
						{ID: "dashboard"},
						{ID: "api"},
						{ID: "sso"},
					},
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, _, err := c.Plans.Get(ctx, tt.give)
				require.NoError(t, err)
				assert.Equal(t, tt.want.ID, result.Data.ID)
				assert.Equal(t, tt.want.Name, result.Data.Name)
				assert.Nil(t, result.Data.Description)
				assert.Empty(t, result.Data.Variants)
				if tt.want.Features == nil {
					assert.Empty(t, result.Data.Features)
				} else {
					assertFeatureIDs(t, result.Data.Features, featureIDs(tt.want.Features))
				}
			})
		}
	})

	t.Run("Users.Get", func(t *testing.T) {
		tests := []struct {
			name       string
			give       grantsy.UserGetParams
			want       grantsy.UserResponse
			wantHasSub bool
		}{
			{
				name: "pro/no expand",
				give: grantsy.UserGetParams{UserID: "user-pro"},
				want: grantsy.UserResponse{
					UserID: "user-pro",
					PlanID: "pro",
				},
			},
			{
				name: "pro/expand plan",
				give: grantsy.UserGetParams{
					UserID: "user-pro",
					Expand: []grantsy.UserExpand{grantsy.UserExpandPlan},
				},
				want: grantsy.UserResponse{
					UserID: "user-pro",
					PlanID: "pro",
					Plan:   &grantsy.Plan{ID: "pro", Name: "Pro"},
				},
			},
			{
				name: "pro/expand features",
				give: grantsy.UserGetParams{
					UserID: "user-pro",
					Expand: []grantsy.UserExpand{grantsy.UserExpandFeatures},
				},
				want: grantsy.UserResponse{
					UserID: "user-pro",
					PlanID: "pro",
					Features: []grantsy.Feature{
						{ID: "dashboard"},
						{ID: "api"},
						{ID: "sso"},
					},
				},
			},
			{
				name: "pro/expand subscription",
				give: grantsy.UserGetParams{
					UserID: "user-pro",
					Expand: []grantsy.UserExpand{grantsy.UserExpandSubscription},
				},
				want: grantsy.UserResponse{
					UserID: "user-pro",
					PlanID: "pro",
				},
				wantHasSub: true,
			},
			{
				name: "pro/expand all",
				give: grantsy.UserGetParams{
					UserID: "user-pro",
					Expand: []grantsy.UserExpand{
						grantsy.UserExpandPlan,
						grantsy.UserExpandFeatures,
						grantsy.UserExpandSubscription,
					},
				},
				want: grantsy.UserResponse{
					UserID: "user-pro",
					PlanID: "pro",
					Plan:   &grantsy.Plan{ID: "pro", Name: "Pro"},
					Features: []grantsy.Feature{
						{ID: "dashboard"},
						{ID: "api"},
						{ID: "sso"},
					},
				},
				wantHasSub: true,
			},
			{
				name: "free/no expand",
				give: grantsy.UserGetParams{UserID: "user-no-sub"},
				want: grantsy.UserResponse{
					UserID: "user-no-sub",
					PlanID: "free",
				},
			},
			{
				name: "free/expand plan",
				give: grantsy.UserGetParams{
					UserID: "user-no-sub",
					Expand: []grantsy.UserExpand{grantsy.UserExpandPlan},
				},
				want: grantsy.UserResponse{
					UserID: "user-no-sub",
					PlanID: "free",
					Plan:   &grantsy.Plan{ID: "free", Name: "Free"},
				},
			},
			{
				name: "free/expand features",
				give: grantsy.UserGetParams{
					UserID: "user-no-sub",
					Expand: []grantsy.UserExpand{grantsy.UserExpandFeatures},
				},
				want: grantsy.UserResponse{
					UserID: "user-no-sub",
					PlanID: "free",
					Features: []grantsy.Feature{
						{ID: "dashboard"},
					},
				},
			},
			{
				name: "free/expand subscription",
				give: grantsy.UserGetParams{
					UserID: "user-no-sub",
					Expand: []grantsy.UserExpand{grantsy.UserExpandSubscription},
				},
				want: grantsy.UserResponse{
					UserID: "user-no-sub",
					PlanID: "free",
				},
			},
			{
				name: "free/expand all",
				give: grantsy.UserGetParams{
					UserID: "user-no-sub",
					Expand: []grantsy.UserExpand{
						grantsy.UserExpandPlan,
						grantsy.UserExpandFeatures,
						grantsy.UserExpandSubscription,
					},
				},
				want: grantsy.UserResponse{
					UserID: "user-no-sub",
					PlanID: "free",
					Plan:   &grantsy.Plan{ID: "free", Name: "Free"},
					Features: []grantsy.Feature{
						{ID: "dashboard"},
					},
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, _, err := c.Users.Get(ctx, tt.give)
				require.NoError(t, err)

				assert.Equal(t, tt.want.UserID, result.Data.UserID)
				assert.Equal(t, tt.want.PlanID, result.Data.PlanID)

				if tt.want.Plan == nil {
					assert.Nil(t, result.Data.Plan)
				} else {
					require.NotNil(t, result.Data.Plan)
					assert.Equal(t, tt.want.Plan.ID, result.Data.Plan.ID)
					assert.Equal(t, tt.want.Plan.Name, result.Data.Plan.Name)
				}

				if tt.want.Features == nil {
					assert.Empty(t, result.Data.Features)
				} else {
					assertFeatureIDs(t, result.Data.Features, featureIDs(tt.want.Features))
				}

				if tt.wantHasSub {
					require.NotNil(t, result.Data.Subscription)
					assertProSubscription(t, result.Data.Subscription)
				} else {
					assert.Nil(t, result.Data.Subscription)
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		t.Run("unauthorized with wrong api key", func(t *testing.T) {
			badClient, err := grantsy.New(baseURL, "wrong-key")
			require.NoError(t, err)

			_, _, err = badClient.Check(ctx, grantsy.CheckParams{
				UserID:  "user-1",
				Feature: "dashboard",
			})
			require.Error(t, err)

			var apiErr *grantsy.APIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, 401, apiErr.Status)
			assert.Equal(t, "Unauthorized", apiErr.Title)
			assert.Equal(t, "Invalid API key", apiErr.Detail)
			assert.NotEmpty(t, apiErr.RequestID)
			assert.Equal(t, grantsy.ErrUnauthorized, grantsy.ErrorType(apiErr.Type))
			assert.Empty(t, apiErr.Fields)
		})

		t.Run("unauthorized without api key", func(t *testing.T) {
			badClient, err := grantsy.New(baseURL, "")
			require.NoError(t, err)

			_, _, err = badClient.Check(ctx, grantsy.CheckParams{
				UserID:  "user-1",
				Feature: "dashboard",
			})
			require.Error(t, err)

			var apiErr *grantsy.APIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, 401, apiErr.Status)
			assert.Equal(t, "Unauthorized", apiErr.Title)
			assert.Equal(t, "Missing API key", apiErr.Detail)
			assert.NotEmpty(t, apiErr.RequestID)
		})

		t.Run("validation error on check access", func(t *testing.T) {
			_, _, err := c.Check(ctx, grantsy.CheckParams{
				UserID:  "user-1",
				Feature: "",
			})
			require.Error(t, err)

			var apiErr *grantsy.APIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, 422, apiErr.Status)
			assert.Equal(t, "Validation Failed", apiErr.Title)
			assert.Equal(t, "One or more fields failed validation", apiErr.Detail)
			assert.NotEmpty(t, apiErr.RequestID)
			assert.Equal(
				t,
				grantsy.ErrValidationFailed,
				grantsy.ErrorType(apiErr.Type),
			)
			require.Len(t, apiErr.Fields, 1)
			assert.Equal(t, "feature", apiErr.Fields[0].Field)
			assert.Equal(t, "Feature is a required field", apiErr.Fields[0].Message)
		})

		t.Run("error message format", func(t *testing.T) {
			badClient, err := grantsy.New(baseURL, "wrong-key")
			require.NoError(t, err)

			_, _, err = badClient.Check(ctx, grantsy.CheckParams{
				UserID:  "user-1",
				Feature: "dashboard",
			})
			require.Error(t, err)
			assert.Contains(
				t,
				err.Error(),
				"grantsy: 401 Unauthorized: Invalid API key",
			)
		})
	})
}
