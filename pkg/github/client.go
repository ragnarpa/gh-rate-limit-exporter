package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v48/github"
	"golang.org/x/oauth2"
)

type Metadata interface {
	Name() string
	Kind() string
}

type App interface {
	Metadata

	ID() int64
	InstallationID() int64
	Base64PrivateKey() string
}

type PAT interface {
	Metadata

	Token() string
}

type RateLimit struct {
	Resource          string
	Limit             int
	Remaining         int
	Reset             time.Time
	AppName           string
	AppKind           string
	AppID             string
	AppInstallationID string
}

func NewRateLimit(m *metadata, resource string, rate *github.Rate) *RateLimit {
	return &RateLimit{
		Resource:          resource,
		Limit:             rate.Limit,
		Remaining:         rate.Remaining,
		Reset:             rate.Reset.Time,
		AppName:           m.name,
		AppKind:           m.kind,
		AppID:             m.id,
		AppInstallationID: m.installationId,
	}
}

type Instrumenter interface {
	Instrument(*http.Client)
}

type metadata struct {
	name           string
	kind           string
	id             string
	installationId string
}

type gitHubClient struct {
	metadata *metadata
	client   *github.Client
}

func NewGitHubClientForApp(app App, base *http.Client) *gitHubClient {
	client := github.NewClient(base)
	metadata := &metadata{
		name:           app.Name(),
		id:             fmt.Sprint(app.ID()),
		installationId: fmt.Sprint(app.InstallationID()),
		kind:           app.Kind(),
	}

	return &gitHubClient{metadata: metadata, client: client}
}

func NewGitHubClientForPAT(pat PAT, base *http.Client) *gitHubClient {
	client := github.NewClient(base)
	metadata := &metadata{name: pat.Name(), kind: pat.Kind()}

	return &gitHubClient{metadata: metadata, client: client}
}

func NewHTTPClientForApp(app App) (*http.Client, error) {
	key, err := base64.StdEncoding.DecodeString(app.Base64PrivateKey())
	if err != nil {
		return nil, err
	}

	itr, err := ghinstallation.New(
		http.DefaultTransport,
		app.ID(),
		app.InstallationID(),
		key,
	)
	if err != nil {
		return nil, err
	}

	return &http.Client{Transport: itr}, nil
}

func NewHTTPClientForPAT(ctx context.Context, pat PAT) *http.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: pat.Token()})
	return oauth2.NewClient(ctx, ts)
}

type resourceRateLimit struct {
	name string
	rate *github.Rate
}

func (c *gitHubClient) RateLimits(ctx context.Context) ([]*RateLimit, error) {
	limits, _, err := c.client.RateLimits(ctx)
	if err != nil {
		return nil, err
	}

	var enrichedLimits []*RateLimit
	for _, r := range []*resourceRateLimit{
		{name: "core", rate: limits.Core},
		{name: "search", rate: limits.Search},
		{name: "graphql", rate: limits.GraphQL},
		{name: "scim", rate: limits.SCIM},
		{name: "source_import", rate: limits.SourceImport},
		{name: "code_scanning_upload", rate: limits.CodeScanningUpload},
		{name: "integration_manifest", rate: limits.IntegrationManifest},
		{name: "actions_runner_registration", rate: limits.ActionsRunnerRegistration},
	} {
		if r.rate != nil {
			enrichedLimits = append(enrichedLimits, NewRateLimit(c.metadata, r.name, r.rate))
		}
	}

	return enrichedLimits, nil
}
