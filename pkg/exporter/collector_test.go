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

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	logger_mocks "github.com/ragnarpa/gh-rate-limit-exporter/logger/mocks"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/github"
	"github.com/stretchr/testify/assert"
)

type instrumenterMock struct{}

func (i *instrumenterMock) Instrument(*http.Client) {}

func GeneratePrivateKey(t *testing.T) string {
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
	return []*github.RateLimit{
		{
			Resource:          rls.resource,
			Limit:             rls.limit,
			Remaining:         rls.remaining,
			AppName:           rls.appName,
			AppKind:           rls.appKind,
			AppID:             rls.appID,
			AppInstallationID: rls.appInstallationID,
		},
	}, nil
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
					Key:            GeneratePrivateKey(t),
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
		Log:          &logger_mocks.Logger{},
	}
}

func TestNewCollector(t *testing.T) {
	t.Run("returns new prepared rate limit metrics collector", func(t *testing.T) {
		cp := newTestCollectorParams()
		c := NewCollector(cp)

		assert.Equal(t, cp.Credentials, c.credentials)
		assert.Equal(t, cp.Factory, c.factory)
		assert.Equal(t, cp.Log, c.log)
		assert.NotNil(t, c.rateLimit)
		assert.NotNil(t, c.rateLimitRemaining)
		assert.NotNil(t, c.rateLimitUsage)
	})
}

func findMetricFamily(name string, metrics []*io_prometheus_client.MetricFamily) *io_prometheus_client.MetricFamily {
	for _, m := range metrics {
		if m.GetName() == name {
			return m
		}
	}

	return nil
}

func TestCollectorStart(t *testing.T) {
	assertContainsRateLimitMetrics := func(t *testing.T, metrics []*io_prometheus_client.MetricFamily) {
		assert.NotNil(t, findMetricFamily("gh_rate_limit_exporter_rate_limit_remaining", metrics))
		assert.NotNil(t, findMetricFamily("gh_rate_limit_exporter_rate_limit_usage", metrics))
		assert.NotNil(t, findMetricFamily("gh_rate_limit_exporter_rate_limit_total", metrics))
	}

	t.Run("starts rate limit metrics collection", func(t *testing.T) {
		cp := newTestCollectorParams()
		c := NewCollector(cp)
		reg := prometheus.NewRegistry()
		reg.MustRegister(c.Collectors()...)

		ctx, cancel := context.WithCancel(context.Background())
		c.Start(ctx)
		<-time.After(2 * time.Second)
		cancel()

		metrics, err := reg.Gather()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertContainsRateLimitMetrics(t, metrics)
	})
}

func TestCollectorSetRateXxx(t *testing.T) {
	t.Parallel()

	rl := &github.RateLimit{
		Resource:          "test-resource",
		Limit:             1000,
		Remaining:         500,
		AppName:           "test-name",
		AppKind:           string(GitHubApp),
		AppID:             "test-app-id",
		AppInstallationID: "test-app-installation-id",
	}

	findLabel := func(name string, labels []*io_prometheus_client.LabelPair) *io_prometheus_client.LabelPair {
		for _, l := range labels {
			if l.GetName() == name {
				return l
			}
		}

		return nil
	}

	assertCorrectValueAndLabels := func(t *testing.T, expected float64, m *io_prometheus_client.Metric) {
		val := m.Gauge.GetValue()
		labels := m.GetLabel()

		assert.Equal(t, expected, val)
		assert.Equal(t, rl.Resource, findLabel(LabelResource, labels).GetValue())
		assert.Equal(t, rl.AppName, findLabel(LabelName, labels).GetValue())
		assert.Equal(t, rl.AppKind, findLabel(LabelType, labels).GetValue())
		assert.Equal(t, rl.AppID, findLabel(LabelAppID, labels).GetValue())
		assert.Equal(t, rl.AppInstallationID, findLabel(LabelAppInstallationID, labels).GetValue())
	}

	t.Run("sets total rate limit", func(t *testing.T) {
		c := NewCollector(CollectorParams{})
		reg := prometheus.NewRegistry()
		reg.MustRegister(c.Collectors()...)

		c.SetRateLimit(rl)

		metrics, err := reg.Gather()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mf := findMetricFamily("gh_rate_limit_exporter_rate_limit_total", metrics)
		m := mf.GetMetric()[0]

		assert.Len(t, mf.GetMetric(), 1)
		assertCorrectValueAndLabels(t, float64(rl.Limit), m)
	})

	t.Run("sets remaining rate limit", func(t *testing.T) {
		c := NewCollector(CollectorParams{})
		reg := prometheus.NewRegistry()
		reg.MustRegister(c.Collectors()...)

		c.SetRateLimitRemaining(rl)

		metrics, err := reg.Gather()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mf := findMetricFamily("gh_rate_limit_exporter_rate_limit_remaining", metrics)
		m := mf.GetMetric()[0]

		assert.Len(t, mf.GetMetric(), 1)
		assertCorrectValueAndLabels(t, float64(rl.Remaining), m)
	})

	t.Run("sets rate limit usage", func(t *testing.T) {
		c := NewCollector(CollectorParams{})
		reg := prometheus.NewRegistry()
		reg.MustRegister(c.Collectors()...)

		c.SetRateLimitUsage(rl)

		metrics, err := reg.Gather()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mf := findMetricFamily("gh_rate_limit_exporter_rate_limit_usage", metrics)
		m := mf.GetMetric()[0]

		assert.Len(t, mf.GetMetric(), 1)
		assertCorrectValueAndLabels(t, float64(0.5), m)
	})
}
