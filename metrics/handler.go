package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type buckets []float64

type HTTPHandlerInstrumenter interface {
	Instrument(handlerName string, handler http.Handler) http.HandlerFunc
}

type httpHandlerInstrumenter struct {
	buckets  buckets
	registry *prometheus.Registry
}

func NewHTTPHandlerInstrumenter(r *prometheus.Registry, b buckets) *httpHandlerInstrumenter {
	return &httpHandlerInstrumenter{buckets: b, registry: r}
}

func (m *httpHandlerInstrumenter) Instrument(handlerName string, handler http.Handler) http.HandlerFunc {
	reg := prometheus.WrapRegistererWith(prometheus.Labels{"handler": handlerName}, m.registry)

	requestsTotal := promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Tracks the number of HTTP requests.",
		}, []string{"method", "code"},
	)
	requestDuration := promauto.With(reg).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Tracks the latencies for HTTP requests.",
			Buckets: m.buckets,
		},
		[]string{"method", "code"},
	)
	requestSize := promauto.With(reg).NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_request_size_bytes",
			Help: "Tracks the size of HTTP requests.",
		},
		[]string{"method", "code"},
	)
	responseSize := promauto.With(reg).NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_response_size_bytes",
			Help: "Tracks the size of HTTP responses.",
		},
		[]string{"method", "code"},
	)

	hf := http.HandlerFunc(
		func(writer http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(writer, r)
		},
	)

	base := promhttp.InstrumentHandlerResponseSize(responseSize, hf)
	base = promhttp.InstrumentHandlerRequestSize(requestSize, base)
	base = promhttp.InstrumentHandlerDuration(requestDuration, base)
	base = promhttp.InstrumentHandlerCounter(requestsTotal, base)

	return base.ServeHTTP
}
