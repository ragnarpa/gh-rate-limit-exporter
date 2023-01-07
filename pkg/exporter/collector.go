package exporter

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/github"
	"go.uber.org/fx"
)

type (
	Instrumenter interface {
		Instrument(*http.Client)
	}

	RateLimitsService interface {
		RateLimits(context.Context) ([]*github.RateLimit, error)
	}

	RateLimitsServiceFactory interface {
		Create(context.Context, *Credential) (RateLimitsService, error)
	}
)

type (
	HttpClientWithPATFactory func(context.Context, github.PAT) *http.Client
	HttpClientWithAppFactory func(github.App) (*http.Client, error)

	RateLimitsServiceFactoryParams struct {
		fx.In

		Instrumenter             Instrumenter
		HttpClientWithPATFactory HttpClientWithPATFactory
		HttpClientWithAppFactory HttpClientWithAppFactory
	}

	rateLimitsServiceFactory struct {
		instrumenter            Instrumenter
		createHTTPClientWithPAT func(context.Context, github.PAT) *http.Client
		createHTTPClientWithApp func(github.App) (*http.Client, error)
	}
)

func NewRateLimitsServiceFactory(p RateLimitsServiceFactoryParams) RateLimitsServiceFactory {
	return &rateLimitsServiceFactory{
		instrumenter:            p.Instrumenter,
		createHTTPClientWithPAT: p.HttpClientWithPATFactory,
		createHTTPClientWithApp: p.HttpClientWithAppFactory,
	}
}

func (f *rateLimitsServiceFactory) Create(ctx context.Context, c *Credential) (RateLimitsService, error) {
	switch c.Type {
	case GitHubApp:
		client, err := f.createHTTPClientWithApp(c)
		if err != nil {
			return nil, err
		}

		f.instrumenter.Instrument(client)

		return github.NewGitHubClientForApp(c, client), nil
	case GitHubPAT:
		base := f.createHTTPClientWithPAT(ctx, c)
		f.instrumenter.Instrument(base)

		return github.NewGitHubClientForPAT(c, base), nil
	default:
		return nil, fmt.Errorf("unknown kind: %v", c.Type)
	}
}

const (
	LabelName              = "name"
	LabelResource          = "resource"
	LabelType              = "type"
	LabelAppID             = "app_id"
	LabelAppInstallationID = "app_installation_id"
)

type (
	Interval int64

	CollectorParams struct {
		fx.In

		Interval     *Interval
		Credentials  []*Credential
		Instrumenter Instrumenter
		Factory      RateLimitsServiceFactory
		Log          logger.Logger
	}

	Collector struct {
		credentials        []*Credential
		rateLimitTotal     *prometheus.GaugeVec
		rateLimitRemaining *prometheus.GaugeVec
		rateLimitUsage     *prometheus.GaugeVec
		interval           *Interval
		factory            RateLimitsServiceFactory
		log                logger.Logger
		mtx                sync.Mutex
		ctx                context.Context
		cancel             context.CancelFunc
	}
)

func NewCollector(p CollectorParams) *Collector {
	const ns = "gh_rate_limit_exporter"
	labels := []string{LabelName, LabelResource, LabelType, LabelAppID, LabelAppInstallationID}

	rateLimit := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "rate_limit_total",
			Help:      "the upper limit of requests within the time unit the rate limit is applied on",
		},
		labels,
	)
	rateLimitRemaining := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "rate_limit_remaining",
			Help:      "the amount of requests you can perform within the time unit the rate limit is applied on",
		},
		labels,
	)
	rateLimitUsage := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "rate_limit_usage",
			Help:      "(total - remaining) / total",
		},
		labels,
	)

	ctx, cancel := context.WithCancel(context.Background())

	return &Collector{
		interval:           p.Interval,
		credentials:        p.Credentials,
		rateLimitTotal:     rateLimit,
		rateLimitRemaining: rateLimitRemaining,
		rateLimitUsage:     rateLimitUsage,
		factory:            p.Factory,
		log:                p.Log,
		ctx:                ctx,
		cancel:             cancel,
	}
}

func (c *Collector) Shutdown() {
	c.cancel()
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.rateLimitTotal.Describe(ch)
	c.rateLimitRemaining.Describe(ch)
	c.rateLimitUsage.Describe(ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Reset the metrics. If metrics collection
	// should fail then we don't report possibly stale values.
	c.rateLimitTotal.Reset()
	c.rateLimitRemaining.Reset()
	c.rateLimitUsage.Reset()

	// Only collect if Done is not yet closed.
	// The context may be closed by Shutdown().
	// If the collector has been shut down then
	// let the gatherer collect reset metrics only.
	if c.ctx.Err() == nil {
		c.collectAll(c.ctx)
	}

	c.rateLimitTotal.Collect(ch)
	c.rateLimitRemaining.Collect(ch)
	c.rateLimitUsage.Collect(ch)
}

func (c *Collector) setRateLimitTotal(rl *github.RateLimit) {
	c.rateLimitTotal.
		WithLabelValues(labels(rl)...).
		Set(float64(rl.Limit))
}

func (c *Collector) setRateLimitRemaining(rl *github.RateLimit) {
	c.rateLimitRemaining.
		WithLabelValues(labels(rl)...).
		Set(float64(rl.Remaining))
}

func (c *Collector) setRateLimitUsage(rl *github.RateLimit) {
	c.rateLimitUsage.
		WithLabelValues(labels(rl)...).
		Set(float64(rl.Limit-rl.Remaining) / float64(rl.Limit))
}

func labels(rl *github.RateLimit) []string {
	return []string{
		rl.AppName,
		rl.Resource,
		rl.AppKind,
		fmt.Sprint(rl.AppID),
		fmt.Sprint(rl.AppInstallationID),
	}
}

func (c *Collector) collectAll(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(len(c.credentials))
	defer wg.Wait()

	for _, credential := range c.credentials {
		appName := credential.AppName
		rls, err := c.factory.Create(ctx, credential)
		if err != nil {
			c.log.Errorf("collector %v: %v", appName, err)
			wg.Done()
			continue
		}

		go func() {
			defer wg.Done()
			if err := c.collectOne(ctx, rls); err != nil {
				c.log.Errorf("collector %v: %v", appName, err)
			}
		}()
	}
}

func (c *Collector) collectOne(ctx context.Context, rls RateLimitsService) error {
	limits, err := rls.RateLimits(ctx)
	if err != nil {
		return err
	}

	for _, rl := range limits {
		c.setRateLimitTotal(rl)
		c.setRateLimitRemaining(rl)
		c.setRateLimitUsage(rl)
	}

	return nil
}
