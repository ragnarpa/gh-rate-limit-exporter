package exporter

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"testing"
	"time"

	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/github"
	"github.com/stretchr/testify/assert"
)

type instrumenterMock struct{}

func (i *instrumenterMock) Instrument(*http.Client) {}

type rateLimitsServiceMock struct {
	resource          string
	limit             int
	remaining         int
	appName           string
	appKind           string
	appID             string
	appInstallationID string
}

func (rls *rateLimitsServiceMock) RateLimits(context.Context) ([]*github.RateLimit, error) {
	limits := []*github.RateLimit{
		{
			Resource:          rls.resource,
			Limit:             rls.limit,
			Remaining:         rls.remaining,
			AppName:           rls.appName,
			AppKind:           rls.appKind,
			AppID:             rls.appID,
			AppInstallationID: rls.appInstallationID,
		},
	}

	return limits, nil
}

type rateLimitsServiceFactoryMock struct {
	service      *rateLimitsServiceMock
	instrumenter Instrumenter
}

func (f *rateLimitsServiceFactoryMock) Create(context.Context, *Credential) (RateLimitsService, error) {
	return f.service, nil
}

func newRateLimitsServiceFactoryParamsMock() RateLimitsServiceFactoryParams {
	return RateLimitsServiceFactoryParams{
		Instrumenter:             &instrumenterMock{},
		HttpClientWithPATFactory: func(ctx context.Context, p github.PAT) *http.Client { return http.DefaultClient },
		HttpClientWithAppFactory: func(a github.App) (*http.Client, error) { return http.DefaultClient, nil },
	}
}

func TestRateLimitsServiceFactory(t *testing.T) {
	t.Parallel()

	t.Run("creates new client with GH App", func(t *testing.T) {
		f := NewRateLimitsServiceFactory(newRateLimitsServiceFactoryParamsMock())

		c, err := f.Create(
			context.Background(),
			&Credential{
				Type:    GitHubApp,
				AppName: "test-app",
				AppCredential: &AppCredential{
					ID:             1,
					InstallationID: 2,
					Key:            generatePrivateKey(t),
				},
			},
		)

		assert.NotNil(t, c)
		assert.NoError(t, err)
	})

	t.Run("creates new client with GH PAT", func(t *testing.T) {
		f := NewRateLimitsServiceFactory(newRateLimitsServiceFactoryParamsMock())

		c, err := f.Create(
			context.Background(),
			&Credential{
				Type:    GitHubPAT,
				AppName: "test-app",
				PAT:     &PAT{Token: "test-token"},
			},
		)

		assert.NotNil(t, c)
		assert.NoError(t, err)
	})

	t.Run("throws on unknown credential type", func(t *testing.T) {
		f := NewRateLimitsServiceFactory(newRateLimitsServiceFactoryParamsMock())

		_, err := f.Create(
			context.Background(),
			&Credential{Type: "unknown", AppName: "test-app"},
		)

		if assert.Error(t, err) {
			assert.EqualError(t, err, "unknown kind: unknown")
		}
	})
}

func TestNewCollector(t *testing.T) {
	t.Run("returns new prepared rate limit metrics collector", func(t *testing.T) {
		cp := newTestCollectorParams()
		c := NewCollector(cp)

		assert.Equal(t, cp.Credentials, c.credentials)
		assert.Equal(t, cp.Factory, c.factory)
		assert.Equal(t, cp.Log, c.log)
		assert.NotNil(t, c.rateLimitTotal)
		assert.NotNil(t, c.rateLimitRemaining)
		assert.NotNil(t, c.rateLimitUsage)
		assert.NotNil(t, c.ctx)
		assert.NotNil(t, c.cancel)
	})
}

func newTestCollectorParams() CollectorParams {
	instrumenter := &instrumenterMock{}
	service := &rateLimitsServiceMock{
		resource:  "test-resource",
		limit:     1000,
		remaining: 500,
		appName:   "test-app",
		appKind:   string(GitHubPAT),
	}
	credentials := []*Credential{
		{Type: Type(service.appKind), AppName: service.appName, PAT: &PAT{Token: "token"}},
	}
	interval := Interval(1 * time.Second)

	return CollectorParams{
		Interval:     &interval,
		Credentials:  credentials,
		Instrumenter: instrumenter,
		Factory:      &rateLimitsServiceFactoryMock{instrumenter: instrumenter, service: service},
		Log:          &logger.NopLogger{},
	}
}

func generatePrivateKey(t *testing.T) string {
	key, err := rsa.GenerateKey(rand.Reader, 128)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: data,
	}

	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(block))
}
