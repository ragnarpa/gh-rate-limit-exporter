package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/exporter"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/github"
	"github.com/ragnarpa/gh-rate-limit-exporter/server"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

type nopLogger struct{}

func (*nopLogger) Infof(format string, args ...any) {}
func (*nopLogger) Warnf(format string, args ...any) {}
func (*nopLogger) Info(args ...any)                 {}
func (*nopLogger) Warn(args ...any)                 {}
func (*nopLogger) Error(args ...any)                {}

type credentialSourceMock struct{}

func (s *credentialSourceMock) Credentials() []*exporter.Credential {
	return []*exporter.Credential{
		{
			Type:    exporter.GitHubPAT,
			AppName: "app-name",
			PAT:     &exporter.PAT{},
		},
	}
}

var rateLimitResponseMock = `
{
	"resources": {
	  "core": {
		"limit": 5000,
		"remaining": 4999,
		"reset": 1372700873,
		"used": 1
	  },
	  "search": {
		"limit": 30,
		"remaining": 18,
		"reset": 1372697452,
		"used": 12
	  },
	  "graphql": {
		"limit": 5000,
		"remaining": 4993,
		"reset": 1372700389,
		"used": 7
	  },
	  "integration_manifest": {
		"limit": 5000,
		"remaining": 4999,
		"reset": 1551806725,
		"used": 1
	  },
	  "code_scanning_upload": {
		"limit": 500,
		"remaining": 499,
		"reset": 1551806725,
		"used": 1
	  }
	},
	"rate": {
	  "limit": 5000,
	  "remaining": 4999,
	  "reset": 1372700873,
	  "used": 1
	}
}
`

type roundTripperMock struct{}

func (*roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	sc := http.StatusOK
	r := &http.Response{
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Status:        http.StatusText(sc),
		StatusCode:    sc,
		Body:          io.NopCloser(strings.NewReader(rateLimitResponseMock)),
		Request:       req,
		ContentLength: int64(len(rateLimitResponseMock)),
		Header:        make(http.Header, 0),
	}

	return r, nil
}

var httpClientMock *http.Client = &http.Client{Transport: &roundTripperMock{}}

func newHttpClientWithAppFactory(github.App) (*http.Client, error) {
	return httpClientMock, nil
}

func newHttpClientWithPATFactory(context.Context, github.PAT) *http.Client {
	return httpClientMock
}

func fatal(t *testing.T, err error) {
	t.Fatalf("unexpected error: %v", err)
}

func findMetricFamily(t *testing.T, name string) *io_prometheus_client.MetricFamily {
	for _, m := range getMetricFamilies(t) {
		if m.GetName() == name {
			return m
		}
	}

	return nil
}

func findMetricByResource(resourceValue string, m []*io_prometheus_client.Metric) *io_prometheus_client.Metric {
	for _, m := range m {
		for _, got := range m.GetLabel() {
			if got.GetName() == "resource" && got.GetValue() == resourceValue {
				return m
			}
		}
	}

	return nil
}

func getMetricFamilies(t *testing.T) []*io_prometheus_client.MetricFamily {
	resp, err := http.Get("http://" + server.Host + ":" + server.Port + "/metrics")
	if err != nil {
		fatal(t, err)
	}

	tp := &expfmt.TextParser{}
	metrics, err := tp.TextToMetricFamilies(resp.Body)
	if err != nil {
		fatal(t, err)
	}

	res := make([]*io_prometheus_client.MetricFamily, len(metrics))
	for _, m := range metrics {
		res = append(res, m)
	}

	return res
}

func usage(remaining, limit float64) float64 {
	return (limit - remaining) / limit
}

func SUT(ctx context.Context, t *testing.T) *fx.App {
	i := exporter.Interval(100 * time.Millisecond)
	app := fx.New(
		module(),
		fx.Replace(&i),
		fx.Replace(fx.Annotate(&nopLogger{}, fx.As(new(logger.Logger)))),
		fx.Replace(fx.Annotate(&credentialSourceMock{}, fx.As(new(exporter.CredentialSource)))),
		fx.Decorate(func() exporter.HttpClientWithAppFactory { return newHttpClientWithAppFactory }),
		fx.Decorate(func() exporter.HttpClientWithPATFactory { return newHttpClientWithPATFactory }),
	)

	if err := app.Start(ctx); err != nil {
		defer func() {
			app.Stop(ctx)
		}()
		fatal(t, err)
	}

	return app
}

func TestModule(t *testing.T) {
	t.Parallel()

	t.Run("fx app starts and stops cleanly", func(t *testing.T) {
		app := fxtest.New(t, module())
		app.RequireStart().RequireStop()
	})

	for _, test := range []struct {
		resource string
		metric   string
		expected float64
	}{
		{resource: "core", metric: "gh_rate_limit_exporter_rate_limit_total", expected: 5000},
		{resource: "core", metric: "gh_rate_limit_exporter_rate_limit_remaining", expected: 4999},
		{resource: "core", metric: "gh_rate_limit_exporter_rate_limit_usage", expected: usage(4999, 5000)},
		{resource: "search", metric: "gh_rate_limit_exporter_rate_limit_total", expected: 30},
		{resource: "search", metric: "gh_rate_limit_exporter_rate_limit_remaining", expected: 18},
		{resource: "search", metric: "gh_rate_limit_exporter_rate_limit_usage", expected: usage(18, 30)},
		{resource: "graphql", metric: "gh_rate_limit_exporter_rate_limit_total", expected: 5000},
		{resource: "graphql", metric: "gh_rate_limit_exporter_rate_limit_remaining", expected: 4993},
		{resource: "graphql", metric: "gh_rate_limit_exporter_rate_limit_usage", expected: usage(4993, 5000)},
		{resource: "integration_manifest", metric: "gh_rate_limit_exporter_rate_limit_total", expected: 5000},
		{resource: "integration_manifest", metric: "gh_rate_limit_exporter_rate_limit_remaining", expected: 4999},
		{resource: "integration_manifest", metric: "gh_rate_limit_exporter_rate_limit_usage", expected: usage(4999, 5000)},
		{resource: "code_scanning_upload", metric: "gh_rate_limit_exporter_rate_limit_total", expected: 500},
		{resource: "code_scanning_upload", metric: "gh_rate_limit_exporter_rate_limit_remaining", expected: 499},
		{resource: "code_scanning_upload", metric: "gh_rate_limit_exporter_rate_limit_usage", expected: usage(499, 500)},
	} {
		t.Run("fx app serves expected metrics", func(t *testing.T) {
			ctx := context.Background()
			app := SUT(ctx, t)
			defer app.Stop(ctx)

			<-time.After(200 * time.Millisecond)

			mf := findMetricFamily(t, test.metric)
			m := findMetricByResource(test.resource, mf.GetMetric())

			assert.Equal(t, test.expected, m.Gauge.GetValue())
		})
	}
}
