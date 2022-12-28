package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HTTPClientInstrumenter interface {
	Instrument(*http.Client)
}

type httpClientInstrumenter struct {
	registry *prometheus.Registry
	counter  *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewHTTPClientInstrumenter(r *prometheus.Registry) *httpClientInstrumenter {
	duration := promauto.With(r).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_client_request_duration_seconds",
			Help:    "HTTP client request latency histogram.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code", "method"},
	)
	counter := promauto.With(r).NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_client_requests_total",
			Help: "HTTP client requests counter.",
		},
		[]string{"code", "method"},
	)

	return &httpClientInstrumenter{registry: r, counter: counter, duration: duration}
}

func (i *httpClientInstrumenter) Instrument(c *http.Client) {
	t := promhttp.InstrumentRoundTripperDuration(i.duration, c.Transport)
	c.Transport = promhttp.InstrumentRoundTripperCounter(i.counter, t)
}
