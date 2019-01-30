package main

import (
	"crypto/tls"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/julienschmidt/httprouter"
	"github.com/wrouesnel/reverse_exporter/api"
	"github.com/wrouesnel/reverse_exporter/api/apisettings"
	"github.com/wrouesnel/reverse_exporter/config"
	"github.com/wrouesnel/reverse_exporter/metricproxy"
	"github.com/wrouesnel/reverse_exporter/version"
	"go.uber.org/zap"
	"gopkg.in/alecthomas/kingpin.v2"
)

// AppConfig represents the total command line application configuration which is
// applied at startup.
type AppConfig struct {
	ConfigFile string
	//MetricsPath string

	ContextPath string
	StaticProxy string

	ListenAddr string
	TLSCert    string
	TLSKey     string
	TLSAuthCA  string

	LogLevel string

	PrintVersion bool
}

func realMain(appConfig AppConfig) int {
	apiConfig := apisettings.APISettings{}
	apiConfig.ContextPath = appConfig.ContextPath

	if appConfig.ConfigFile == "" {
		zap.L().Fatal("No app config specified.")
	}

	reverseConfig, err := config.LoadFromFile(appConfig.ConfigFile)
	if err != nil {
		zap.L().Fatal("Could not parse configuration file", zap.Error(err))
	}

	// Setup the web UI
	router := httprouter.New()
	router = api.NewAPIv1(apiConfig, router)

	zap.L().Debug("Begin initializing reverse proxy backends")
	initializedPaths := make(map[string]http.Handler)
	for _, rp := range reverseConfig.ReverseExporters {
		if rp.Path == "" {
			zap.L().Fatal("Blank exporter paths are not allowed.")
		}

		if _, found := initializedPaths[rp.Path]; found {
			zap.L().Fatal("Exporter paths must be unique", zap.String("already exists", rp.Path))
		}

		proxyHandler, perr := metricproxy.NewMetricReverseProxy(rp)
		if perr != nil {
			zap.L().Fatal("Error initializing reverse proxy for path", zap.String("path", rp.Path))
		}

		router.Handler("GET", apiConfig.WrapPath(rp.Path), proxyHandler)

		initializedPaths[rp.Path] = proxyHandler
	}
	zap.L().Debug("Finished initializing reverse proxy backends")
	zap.L().Info("Initialized backends", zap.Int("num_reverse_endpoints", len(reverseConfig.ReverseExporters)))

	zap.L().Info("Starting HTTP server")

	listener, err := uniListen(appConfig.ListenAddr)
	if err != nil {
		zap.L().Fatal("Startup failed for a listener", zap.Error(err))
	}
	zap.L().Info("Listening on", zap.String("addr", appConfig.ListenAddr))

	srv := http.Server{
		Handler: router,
	}

	listenerErrs := make(chan error)

	if appConfig.TLSCert != "" && appConfig.TLSKey != "" {
		if appConfig.TLSAuthCA != "" {
			tlsConfig, err := tlsAuthConfig(appConfig.TLSAuthCA)
			if err != nil {
				zap.L().Error("Error creating TLS config", zap.Error(err))
				return 1
			}

			srv.TLSConfig = tlsConfig
		}

		srv.TLSConfig.MinVersion = tls.VersionTLS12

		go func() {
			err := srv.ServeTLS(listener, appConfig.TLSCert, appConfig.TLSKey)
			listenerErrs <- err
		}()
	} else {
		go func() {
			err := srv.Serve(listener)
			listenerErrs <- err
		}()
	}

	// Setup signal wait for shutdown
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// If a listener fails while it's listening, we'd like to panic and shutdown
	// since it shouldn't really happen.
	select {
	case sig := <-shutdownCh:
		zap.L().Info("Terminating on signal", zap.Stringer("signal", sig))
		return 0
	case listenerErr := <-listenerErrs:
		zap.L().Fatal("Terminating due to listener shutdown", zap.Error(listenerErr))
		return 1 // just to satisfy compiler
	}
}

func main() {
	appConfig := AppConfig{}

	app := kingpin.New("reverse_exporter", "Logical-decoding Prometheus exporter reverse proxy")

	app.Flag("config.file", "Path to the configuration file").
		Default("reverse_exporter.yml").StringVar(&appConfig.ConfigFile)
	app.Flag("http.context-path", "Context-path to be globally applied to the configured proxies").
		StringVar(&appConfig.ContextPath)
	app.Flag("http.listen-addr", "Listen address").
		Default("tcp://0.0.0.0:9998").StringVar(&appConfig.ListenAddr)
	app.Flag("tls.cert", "Path to certificate file to be used for TLS listener. No TLS if empty.").
		Default("").StringVar(&appConfig.TLSCert)
	app.Flag("tls.key", "Path to private key file to be used for TLS listener. No TLS if empty.").
		Default("").StringVar(&appConfig.TLSKey)
	app.Flag("tls.auth.ca", "Path to CA cert file to be used for TLS client cert auth. No authentication if empty.").
		Default("").StringVar(&appConfig.TLSAuthCA)
	app.Flag("log.level", "Only log messages with the given severity or above. Valid levels: [debug info warn error dpanic panic fatal]").
		Default("info").StringVar(&appConfig.LogLevel)
	app.Version(version.Version)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	prepareLogger(appConfig.LogLevel)

	os.Exit(realMain(appConfig))
}
