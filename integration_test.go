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
  plans:
    - id: free
      name: Free
      features: [dashboard]
    - id: pro
      name: Pro
      features: [dashboard, api, sso]
  features:
    - id: dashboard
      name: Dashboard
      description: Basic dashboard access
    - id: api
      name: API Access
      description: REST API access
    - id: sso
      name: Single Sign-On
      description: SSO integration
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
				ExposedPorts: []string{"8080/tcp"},
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

func TestIntegration(t *testing.T) {
	client, baseURL := startGrantsy(t)
	ctx := context.Background()

	t.Run("check feature access allowed", func(t *testing.T) {
		result, err := client.CheckAccess(ctx, grantsy.CheckAccessParams{
			UserID:  "user-1",
			Feature: "dashboard",
		})
		require.NoError(t, err)
		assert.Equal(t, true, result.Allowed)
		assert.Equal(t, "dashboard", result.Feature)
		assert.Equal(t, "free", result.Plan)
	})

	t.Run("check feature access denied", func(t *testing.T) {
		result, err := client.CheckAccess(ctx, grantsy.CheckAccessParams{
			UserID:  "user-1",
			Feature: "api",
		})
		require.NoError(t, err)
		assert.Equal(t, false, result.Allowed)
		assert.Equal(t, "api", result.Feature)
	})

	t.Run("list features", func(t *testing.T) {
		result, err := client.ListFeatures(ctx, grantsy.ListFeaturesParams{
			UserID: "user-1",
		})
		require.NoError(t, err)
		assert.Equal(t, "free", result.Plan)
		assert.Equal(t, []string{"dashboard"}, result.Features)
	})

	t.Run("list plans", func(t *testing.T) {
		result, err := client.ListPlans(ctx, grantsy.ListPlansParams{})
		require.NoError(t, err)
		assert.Len(t, result.Plans, 2)
	})

	t.Run("list plans with expanded features", func(t *testing.T) {
		result, err := client.ListPlans(ctx, grantsy.ListPlansParams{
			Expand: "features",
		})
		require.NoError(t, err)
		assert.Len(t, result.AllFeatures, 3)
	})

	t.Run("get subscription for user with subscription", func(t *testing.T) {
		result, err := client.GetSubscription(ctx, grantsy.GetSubscriptionParams{
			UserID: "user-pro",
		})
		require.NoError(t, err)
		assert.Equal(t, true, result.HasSubscription)
		assert.Equal(t, "active", result.Status)
		assert.Equal(t, "pro", result.Plan)

		assert.Equal(t, grantsy.ProviderLemonSqueezy, result.Raw.Provider)
		lsData, err := result.Raw.Data.AsLemonSqueezySubscriptionDTO()
		require.NoError(t, err)
		assert.Equal(t, "active", lsData.Status)
		assert.Equal(t, "Active", lsData.StatusFormatted)
		assert.Equal(t, 12345, lsData.ProductID)
		assert.Equal(t, "Pro", lsData.ProductName)
	})

	t.Run("check feature access for subscribed user", func(t *testing.T) {
		result, err := client.CheckAccess(ctx, grantsy.CheckAccessParams{
			UserID:  "user-pro",
			Feature: "api",
		})
		require.NoError(t, err)
		assert.Equal(t, true, result.Allowed)
		assert.Equal(t, "pro", result.Plan)
	})

	t.Run("list features for subscribed user", func(t *testing.T) {
		result, err := client.ListFeatures(ctx, grantsy.ListFeaturesParams{
			UserID: "user-pro",
		})
		require.NoError(t, err)
		assert.Equal(t, "pro", result.Plan)
		assert.ElementsMatch(
			t,
			[]string{"dashboard", "api", "sso"},
			result.Features,
		)
	})

	t.Run("get subscription for user without subscription", func(t *testing.T) {
		result, err := client.GetSubscription(ctx, grantsy.GetSubscriptionParams{
			UserID: "user-no-sub",
		})
		require.NoError(t, err)
		assert.Equal(t, false, result.HasSubscription)
		assert.Equal(t, "free", result.Plan)
		assert.Equal(t, []string{"dashboard"}, result.Features)
	})

	t.Run("unauthorized with wrong api key", func(t *testing.T) {
		badClient, err := grantsy.New(baseURL, "wrong-key")
		require.NoError(t, err)

		_, err = badClient.CheckAccess(ctx, grantsy.CheckAccessParams{
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
		assert.Equal(t, grantsy.ErrUnauthorized, apiErr.Type)
		assert.Empty(t, apiErr.Fields)
	})

	t.Run("unauthorized without api key", func(t *testing.T) {
		badClient, err := grantsy.New(baseURL, "")
		require.NoError(t, err)

		_, err = badClient.CheckAccess(ctx, grantsy.CheckAccessParams{
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
		_, err := client.CheckAccess(ctx, grantsy.CheckAccessParams{
			UserID: "user-1",
		})
		require.Error(t, err)

		var apiErr *grantsy.APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 422, apiErr.Status)
		assert.Equal(t, "Validation Failed", apiErr.Title)
		assert.Equal(t, "One or more fields failed validation", apiErr.Detail)
		assert.NotEmpty(t, apiErr.RequestID)
		assert.Equal(t, grantsy.ErrValidationFailed, apiErr.Type)
		require.Len(t, apiErr.Fields, 1)
		assert.Equal(t, "feature", apiErr.Fields[0].Field)
		assert.Equal(t, "Feature is a required field", apiErr.Fields[0].Message)
	})

	t.Run("validation error on list features", func(t *testing.T) {
		_, err := client.ListFeatures(ctx, grantsy.ListFeaturesParams{})
		require.Error(t, err)

		var apiErr *grantsy.APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 422, apiErr.Status)
		assert.Equal(t, "Validation Failed", apiErr.Title)
		assert.Equal(t, grantsy.ErrValidationFailed, apiErr.Type)
		require.Len(t, apiErr.Fields, 1)
		assert.Equal(t, "UserID is a required field", apiErr.Fields[0].Message)
	})

	t.Run("validation error on get subscription", func(t *testing.T) {
		_, err := client.GetSubscription(ctx, grantsy.GetSubscriptionParams{})
		require.Error(t, err)

		var apiErr *grantsy.APIError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 422, apiErr.Status)
		assert.Equal(t, "Validation Failed", apiErr.Title)
		assert.Equal(t, grantsy.ErrValidationFailed, apiErr.Type)
		require.Len(t, apiErr.Fields, 1)
		assert.Equal(t, "UserID is a required field", apiErr.Fields[0].Message)
	})

	t.Run("error message format", func(t *testing.T) {
		badClient, err := grantsy.New(baseURL, "wrong-key")
		require.NoError(t, err)

		_, err = badClient.CheckAccess(ctx, grantsy.CheckAccessParams{
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
}
