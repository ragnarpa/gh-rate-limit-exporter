package exporter

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

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

	rateLimitsServiceFactory struct {
		instrumenter            Instrumenter
		createHTTPClientWithPAT func(context.Context, github.PAT) *http.Client
		createHTTPClientWithApp func(github.App) (*http.Client, error)
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

	Collector struct {
		credentials        []*Credential
		rateLimit          *prometheus.GaugeVec
		rateLimitRemaining *prometheus.GaugeVec
		rateLimitUsage     *prometheus.GaugeVec
		interval           *Interval
		factory            RateLimitsServiceFactory
		log                logger.Logger
		once               sync.Once
	}

	CollectorParams struct {
		fx.In

		Interval     *Interval
		Credentials  []*Credential
		Instrumenter Instrumenter
		Factory      RateLimitsServiceFactory
		Log          logger.Logger
	}
)

func NewCollector(params CollectorParams) *Collector {
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

	return &Collector{
		interval:           params.Interval,
		credentials:        params.Credentials,
		rateLimit:          rateLimit,
		rateLimitRemaining: rateLimitRemaining,
		rateLimitUsage:     rateLimitUsage,
		factory:            params.Factory,
		log:                params.Log,
	}
}

func (c *Collector) Collectors() []prometheus.Collector {
	return []prometheus.Collector{c.rateLimit, c.rateLimitRemaining, c.rateLimitUsage}
}

func (c *Collector) SetRateLimit(rl *github.RateLimit) {
	c.rateLimit.
		WithLabelValues(labels(rl)...).
		Set(float64(rl.Limit))
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

func (c *Collector) SetRateLimitRemaining(rl *github.RateLimit) {
	c.rateLimitRemaining.
		WithLabelValues(labels(rl)...).
		Set(float64(rl.Remaining))
}

func (c *Collector) SetRateLimitUsage(rl *github.RateLimit) {
	c.rateLimitUsage.
		WithLabelValues(labels(rl)...).
		Set(float64(rl.Limit-rl.Remaining) / float64(rl.Limit))
}

func (c *Collector) Start(ctx context.Context) {
	c.once.Do(
		func() {
			go func() {
				ticker := time.NewTicker(time.Duration(*c.interval))

				for {
					select {
					case <-ctx.Done():
						ticker.Stop()
						return
					case <-ticker.C:
						c.CollectAll(ctx)
					}
				}
			}()
		},
	)
}

func (c *Collector) CollectAll(ctx context.Context) {
	for _, credential := range c.credentials {
		rls, err := c.factory.Create(ctx, credential)
		if err != nil {
			c.log.Error(err)
			continue
		}

		if err := c.CollectOne(ctx, rls); err != nil {
			c.log.Error(err)
		}
	}
}

func (c *Collector) CollectOne(ctx context.Context, rls RateLimitsService) error {
	limits, err := rls.RateLimits(ctx)
	if err != nil {
		return err
	}

	for _, rl := range limits {
		c.SetRateLimit(rl)
		c.SetRateLimitRemaining(rl)
		c.SetRateLimitUsage(rl)
	}

	return nil
}
