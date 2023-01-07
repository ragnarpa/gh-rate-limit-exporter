package exporter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestMetricsHandlerServeHTTP(t *testing.T) {
	t.Run("implements http.Handler and returns 200", func(t *testing.T) {
		c := NewCollector(CollectorParams{})
		reg := prometheus.NewRegistry()
		var h http.Handler = NewMetricsHandler(c, reg)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(""))
		h.ServeHTTP(rr, req)

		assert.Equal(t, 200, rr.Result().StatusCode)
	})
}
