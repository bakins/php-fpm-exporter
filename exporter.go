package exporter

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Exporter handles serving the metrics
type Exporter struct {
	addr            string
	targets         map[string]*url.URL
	logger          *zap.Logger
	metricsEndpoint string
}

// OptionsFunc is a function passed to new for setting options on a new Exporter.
type OptionsFunc func(*Exporter) error

// New creates an exporter.
func New(options ...OptionsFunc) (*Exporter, error) {
	e := &Exporter{
		addr:    ":9090",
		targets: make(map[string]*url.URL),
	}

	for _, f := range options {
		if err := f(e); err != nil {
			return nil, errors.Wrap(err, "failed to set options")
		}
	}

	if e.logger == nil {
		l, err := NewLogger()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create logger")
		}
		e.logger = l
	}

	if len(e.targets) == 0 {
		u, _ := url.Parse("http://localhost:9000/status")
		e.targets["default"] = u
	}
	return e, nil
}

// SetLogger creates a function that will set the logger.
// Generally only used when create a new Exporter.
func SetLogger(l *zap.Logger) func(*Exporter) error {
	return func(e *Exporter) error {
		e.logger = l
		return nil
	}
}

// SetAddress creates a function that will set the listening address.
// Generally only used when create a new Exporter.
func SetAddress(addr string) func(*Exporter) error {
	return func(e *Exporter) error {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return errors.Wrapf(err, "invalid address")
		}
		e.addr = net.JoinHostPort(host, port)
		return nil
	}
}

// SetTargets creates a function that will set the target URL's for scraping
// Generally only used when create a new Exporter.
func SetTargets(in map[string]string) func(*Exporter) error {
	return func(e *Exporter) error {
		targets := make(map[string]*url.URL)
		for k, v := range in {
			u, err := url.Parse(v)
			if err != nil {
				return errors.Wrap(err, "failed to parse url")
			}
			targets[k] = u
		}
		e.targets = targets
		return nil
	}
}

// SetMetricsEndpoint sets the path under which to expose metrics.
// Generally only used when create a new Exporter.
func SetMetricsEndpoint(path string) func(*Exporter) error {
	return func(e *Exporter) error {
		if path == "" || path == "/" {
			return nil
		}
		e.metricsEndpoint = path
		return nil
	}
}

var healthzOK = []byte("ok\n")

func (e *Exporter) healthz(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write(healthzOK)
}

// Run starts the http server and collecting metrics. It generally does not return.
func (e *Exporter) Run() error {

	c := e.newCollector()
	if err := prometheus.Register(c); err != nil {
		return errors.Wrap(err, "failed to register metrics")
	}
	prometheus.Unregister(prometheus.NewProcessCollector(os.Getpid(), ""))
	prometheus.Unregister(prometheus.NewGoCollector())

	http.HandleFunc("/healthz", e.healthz)
	http.Handle(e.metricsEndpoint, promhttp.Handler())

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
			<head><title>php-fpm exporter</title></head>
			<body>
			<h1>php-fpm exporter</h1>
			<p><a href="` + e.metricsEndpoint + `">Metrics</a></p>
			</body>
			</html>`))
	})

	stopChan := make(chan os.Signal)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	srv := &http.Server{Addr: e.addr}
	var g errgroup.Group

	g.Go(func() error {
		// TODO: allow TLS
		return srv.ListenAndServe()
	})

	g.Go(func() error {
		<-stopChan
		// XXX: should shutdown time be configurable?
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		return nil
	})

	if err := g.Wait(); err != http.ErrServerClosed {
		return errors.Wrap(err, "failed to run server")
	}

	return nil
}
