package main

import (
	"context"
	_ "embed"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	prommodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/exporter"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/github"
	"github.com/ragnarpa/gh-rate-limit-exporter/server"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

//go:embed testdata/rate-limit-response.json
var rateLimitResponse string

type roundTripperMock struct{}

func (*roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	sc := http.StatusOK
	r := &http.Response{
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Status:        http.StatusText(sc),
		StatusCode:    sc,
		Body:          io.NopCloser(strings.NewReader(rateLimitResponse)),
		Request:       req,
		ContentLength: int64(len(rateLimitResponse)),
		Header:        make(http.Header, 0),
	}

	return r, nil
}

var httpClientMock *http.Client = &http.Client{Transport: &roundTripperMock{}}

func newHttpClientWithApp(github.App) (*http.Client, error) {
	return httpClientMock, nil
}

func newHttpClientWithPAT(context.Context, github.PAT) *http.Client {
	return httpClientMock
}

//go:embed testdata/test-credentials.yml
var credentials string

func sut(ctx context.Context, t *testing.T) *fx.App {
	cwd, err := os.Getwd()
	if err != nil {
		fatal(t, err)
	}

	fs := &afero.Afero{Fs: afero.NewMemMapFs()}
	fs.MkdirAll(cwd, 0700)
	fs.WriteFile(filepath.Join(cwd, exporter.FileCredentialFileName), []byte(credentials), 0600)

	i := exporter.Interval(100 * time.Millisecond)

	app := fx.New(
		module(),
		fx.Replace(&i),
		fx.Replace(fx.Annotate(fs, fx.As(new(afero.Fs)))),
		fx.Replace(fx.Annotate(&logger.NopLogger{}, fx.As(new(logger.Logger)))),
		fx.Decorate(func() exporter.HttpClientWithAppFactory { return newHttpClientWithApp }),
		fx.Decorate(func() exporter.HttpClientWithPATFactory { return newHttpClientWithPAT }),
	)

	if err := app.Start(ctx); err != nil {
		defer app.Stop(ctx)
		fatal(t, err)
	}

	return app
}

func TestModule(t *testing.T) {
	t.Parallel()

	t.Run("fx app starts and stops cleanly", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			fatal(t, err)
		}

		fs := &afero.Afero{Fs: afero.NewMemMapFs()}
		fs.MkdirAll(cwd, 0700)
		fs.WriteFile(filepath.Join(cwd, exporter.FileCredentialFileName), []byte(""), 0600)

		app := fxtest.New(
			t,
			module(),
			fx.Replace(fx.Annotate(fs, fx.As(new(afero.Fs)))),
			fx.Replace(fx.Annotate(&logger.NopLogger{}, fx.As(new(logger.Logger)))),
		)

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
			app := sut(ctx, t)
			defer app.Stop(ctx)

			<-time.After(200 * time.Millisecond)

			mf := findMetricFamily(t, test.metric)
			m := findMetricByResource(test.resource, mf.GetMetric())

			assert.Equal(t, test.expected, m.Gauge.GetValue())
		})
	}
}

func fatal(t *testing.T, err error) {
	t.Fatalf("unexpected error: %v", err)
}

func findMetricFamily(t *testing.T, name string) *prommodel.MetricFamily {
	for _, m := range getMetricFamilies(t) {
		if m.GetName() == name {
			return m
		}
	}

	return nil
}

func findMetricByResource(name string, m []*prommodel.Metric) *prommodel.Metric {
	for _, m := range m {
		for _, l := range m.GetLabel() {
			if l.GetName() == "resource" && l.GetValue() == name {
				return m
			}
		}
	}

	return nil
}

func getMetricFamilies(t *testing.T) []*prommodel.MetricFamily {
	resp, err := http.Get("http://localhost:" + server.Port + "/metrics")
	if err != nil {
		fatal(t, err)
	}

	tp := &expfmt.TextParser{}
	mfamilies, err := tp.TextToMetricFamilies(resp.Body)
	if err != nil {
		fatal(t, err)
	}

	res := make([]*prommodel.MetricFamily, len(mfamilies))
	for _, mf := range mfamilies {
		res = append(res, mf)
	}

	return res
}

func usage(remaining, limit float64) float64 {
	return (limit - remaining) / limit
}
