package metricproxy

import (
	"context"
	"errors"
	"net/url"

	"github.com/prometheus/common/model"
	"github.com/wrouesnel/reverse_exporter/config"
	"go.uber.org/zap"

	"net/http"
	"time"

	"github.com/abbot/go-http-auth"
	dto "github.com/prometheus/client_model/go"
)

// nolint: golint
var (
	ErrNameFieldOverrideAttempted = errors.New("cannot override name field with additional labels")
	ErrFileProxyScrapeError       = errors.New("file proxy file read failed")
	ErrUnknownExporterType        = errors.New("cannot configure unknown exporter type")
	ErrExporterNameUsedTwice      = errors.New("cannot use the same exporter name twice for one endpoint")
)

// MetricProxy presents an interface which allows a context-cancellable scrape of a backend proxy
type MetricProxy interface {
	// Scrape returns the metrics.
	Scrape(ctx context.Context, values url.Values) ([]*dto.MetricFamily, error)
}

// NewMetricReverseProxy initializes a new reverse proxy from the given configuration.
func NewMetricReverseProxy(exporter config.ReverseExporter) (http.Handler, error) {
	log := zap.L().With(zap.String("path", exporter.Path)) // nolint: vetshadow

	// Initialize a basic reverse proxy
	backend := &ReverseProxyEndpoint{
		metricPath: exporter.Path,
		backends:   make([]MetricProxy, 0),
	}
	backend.handler = backend.serveMetricsHTTP

	usedNames := make(map[string]struct{})

	// Start adding backends
	for _, exporter := range exporter.Exporters {
		var newExporter MetricProxy

		baseExporter := exporter.(config.BaseExporter).GetBaseExporter()
		log := log.With(zap.String("name", baseExporter.Name)) // nolint: vetshadow

		switch e := exporter.(type) {
		case config.FileExporterConfig:
			log.Debug("Adding new file exporter proxy")
			newExporter = newFileProxy(&e)
		case config.ExecExporterConfig:
			log.Debug("Adding new exec exporter proxy")
			newExporter = newExecProxy(&e)
		case config.ExecCachingExporterConfig:
			log.Debug("Adding new caching exec exporter proxy")
			newExporter = newExecCachingProxy(&e)
		case config.HTTPExporterConfig:
			log.Debug("Adding new http exporter proxy")
			newExporter = &netProxy{
				address:            e.Address,
				deadline:           time.Duration(e.Timeout),
				forwardQueryParams: e.ForwardURLParams,
			}
		default:
			log.Error("Unknown proxy configuration item found", zap.Reflect("item", e))
			return nil, ErrUnknownExporterType
		}

		// Got exporter, now add a rewrite proxy in front of it
		labels := make(model.LabelSet)

		// Keep track of exporter name use to pre-empt collisions
		if _, found := usedNames[baseExporter.Name]; !found {
			usedNames[baseExporter.Name] = struct{}{}
		} else {
			log.Error("Exporter name re-use even if rewrite is disabled is not allowed")
			return nil, ErrExporterNameUsedTwice
		}

		// If not rewriting, log it.
		if !baseExporter.NoRewrite {
			labels[reverseProxyNameLabel] = model.LabelValue(baseExporter.Name)
		} else {
			log.Debug("Disabled explicit exporter name for", zap.String("name", baseExporter.Name))
		}

		// Set the additional labels.
		for k, v := range baseExporter.Labels {
			if k == reverseProxyNameLabel {
				return nil, ErrNameFieldOverrideAttempted
			}
			labels[model.LabelName(k)] = model.LabelValue(v)
		}

		// Configure the rewriting proxy shim.
		rewriteProxy := &rewriteProxy{
			proxy:  newExporter,
			labels: labels,
		}

		// Add the new backend to the endpoint
		backend.backends = append(backend.backends, rewriteProxy)
	}

	// Process the auth configuration
	switch exporter.AuthType {
	case config.AuthTypeNone:
		log.Debug("No authentication for endpoint")
	case config.AuthTypeBasic:
		log.Debug("Adding basic auth to endpoint")

		provider := auth.HtpasswdFileProvider(exporter.HtPasswdFile)
		authenticator := auth.NewBasicAuthenticator(authRealm, provider)

		realHandler := backend.handler
		authHandler := func(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
			realHandler(w, &r.Request)
		}
		backend.handler = authenticator.Wrap(authHandler)

	default:
		log.Error("Unknown auth-type specified:", zap.String("type", string(exporter.AuthType)))
	}

	return backend, nil
}
