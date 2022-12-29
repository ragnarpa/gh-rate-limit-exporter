# gh-rate-limit-exporter

The exporter you longed for to expose GitHub API rate limits as Prometheus metrics.

Why bother? Building your pipelines with GitHub Actions will most likely require you to integrate with GitHub API. The more your business and pipelines grow the more you send requests toward GitHub API. Using GitHub API comes free only until a certain limit and beyond that limit, you will be throttled. This tool aims to give you the visibility on how far you are from the limits with your GitHub Apps and GitHub PATs (autheticated users). Throttling is applied at the authenticated user level.

This tool supports exporting the rate limits for GitHub App credentials and the classic and fine-grained PATs.

To learn more about GitHub API rate limits go [here](https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#rate-limiting).

## How to use

By default, the exporter expects to find credentials (GitHub App or PAT) in credentials.yml in current working directory.

```yaml
my-github-app-name:
	type: gh-app
	appId: <app id (integer) goes here>
	installationId: <installation id (integer) goes here>
	key: <base64 encoded private key goes here>
my-pat-name:
	type: gh-pat
	token: <PAT goes here>
```

You can have as many GitHub credentials in credentials.yml as you want. **gh-rate-limit-exporter** will fetch the rate limit usage for every credential and exposes it on `http://localhost:8080/metrics`.

But I do not want to store my GitHub credentials in credentials.yml!

In this case you need to create a new Go module and write a bit of code in Go. For the sake of example let's assume that you want to consume the credentials directly from the process memory. For that do the following.

- Create a new Go module
- `go get github.com/ragnarpa/gh-rate-limit-exporter`
- Create `main.go` with following content

```go
package main

import (
	"github.com/ragnarpa/gh-rate-limit-exporter/logger"
	"github.com/ragnarpa/gh-rate-limit-exporter/metrics"
	"github.com/ragnarpa/gh-rate-limit-exporter/pkg/exporter"
	"github.com/ragnarpa/gh-rate-limit-exporter/server"
	"go.uber.org/fx"
)

type InMemoryCredentialSource struct{}

func (s *InMemoryCredentialSource) Credentials() []*exporter.Credential {
	return []*exporter.Credential{
		{
			Type:    exporter.GitHubPAT,
			AppName: "app-name",
			PAT:     &exporter.PAT{Token: "dG9rZW4K"},
		},
	}
}

func main() {
	fx.New(
		// Replace the default implementation (credentials.json) provided by
		// gh-rate-limit-exporter with your own in-memory credential Source.
		fx.Replace(
			fx.Annotate(
				&InMemoryCredentialSource{},
				fx.As(new(exporter.CredentialSource)),
			),
		),
		logger.Module(),
		metrics.Module(),
		exporter.Module(),
		server.Module(),
		fx.NopLogger,
	).Run()
}
```

After running your Go program (`go run main.go`) you should see `... 401 Bad credentials ...` error messages in the logs.

The `401` error is expected as the example is using a made-up credential.

This of course, is a very simplistic example and it should be used only for toying on your workstation locally.

If you would like to consume credentials from some sort of credential provider, then feel free to write an implementation that would match the `exporter.CredentialSource` interface and inject it in a similar way as it is done with `InMemoryCredentialSource` in the example. If the implementation is generic enough and the community would benefit from it, then please consider creating a pull request.

## Container image

You can build the container image with the default implementation (credentials.yml) and then run the exporter within a container.

```shell
docker build -t gh-rate-limit-exporter .
docker run --rm -v /path/to/credentials.yml:/credentials.yml -p 8080:8080 gh-rate-limit-exporter 
```

Navigate to `http://localhost:8080/metrics` and after 30 seconds you should see GitHub API rate limit usage exposed as Prometheus metrics.

## Metrics

- gh_rate_limit_exporter_rate_limit_remaining - the amount of requests you can perform within the time unit the rate limit is applied on
- gh_rate_limit_exporter_rate_limit_total - the upper limit of requests within the time unit the rate limit is applied on
- gh_rate_limit_exporter_rate_limit_usage - (total - remaining) / total

To find out how to scrape Prometheus metrics, please go [here](https://prometheus.io/docs/prometheus/latest/getting_started/).