package exporter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/github"
	"github.com/stretchr/testify/assert"
)

func TestNewMetricsHandler(t *testing.T) {
	t.Run("registers collectors", func(t *testing.T) {
		r := prometheus.NewRegistry()
		c := NewCollector(CollectorParams{})

		NewMetricsHandler(c, r)

		for _, c := range c.Collectors() {
			assert.True(t, r.Unregister(c))
		}

		if len(c.Collectors()) < 1 {
			assert.Fail(t, "no collectors")
		}
	})
}

func TestMetricsHandlerServeHTTP(t *testing.T) {
	t.Run("implements http.Handler and serves metrics", func(t *testing.T) {
		c := NewCollector(CollectorParams{})
		var h http.Handler = NewMetricsHandler(c, prometheus.NewRegistry())
		rl := &github.RateLimit{
			Resource:          "resource",
			Limit:             1000,
			Remaining:         1000,
			Reset:             time.Now(),
			AppName:           "name",
			AppKind:           "kind",
			AppID:             "id",
			AppInstallationID: "installationid",
		}

		c.SetRateLimit(rl)
		c.SetRateLimitRemaining(rl)
		c.SetRateLimitUsage(rl)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(""))
		h.ServeHTTP(rr, req)

		tp := &expfmt.TextParser{}
		mfs, err := tp.TextToMetricFamilies(rr.Result().Body)

		assert.NoError(t, err)
		assert.Len(t, mfs, 3)
	})
}
