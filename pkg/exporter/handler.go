package exporter

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsHandler struct {
	registry *prometheus.Registry
}

func NewMetricsHandler(c *Collector, r *prometheus.Registry) *MetricsHandler {
	r.MustRegister(c.Collectors()...)

	return &MetricsHandler{r}
}

func (h *MetricsHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	promhttp.HandlerFor(
		h.registry,
		promhttp.HandlerOpts{EnableOpenMetrics: true},
	).ServeHTTP(w, req)
}
