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

    "go.uber.org/zap"

    "github.com/pkg/errors"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "golang.org/x/sync/errgroup"
)

// Exporter handles serving the metrics
type Exporter struct {
    addr     string
    endpoint *url.URL
    origEndpoint string
    logger   *zap.Logger
    clientcert string
    clientkey string
}

// OptionsFunc is a function passed to new for setting options on a new Exporter.
type OptionsFunc func(*Exporter) error

// New creates an exporter.
func New(options ...OptionsFunc) (*Exporter, error) {
    e := &Exporter{
        addr: ":9090",
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

    if e.endpoint == nil {
        u, _ := url.Parse("http://localhost:9000/status")
        e.endpoint = u
    }
    e.origEndpoint = e.endpoint.String()
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

// SetEndpoint creates a function that will set the URL endpoint to contact
// php-fpm.
// Generally only used when create a new Exporter.
func SetEndpoint(rawurl string) func(*Exporter) error {
    return func(e *Exporter) error {
        u, err := url.Parse(rawurl)
        if err != nil {
            return errors.Wrap(err, "failed to parse url")
        }
        e.endpoint = u
        return nil
    }
}


// SetEndpoint creates a function that will set the URL endpoint to contact
// php-fpm.
// Generally only used when create a new Exporter.
func SetCerts(cert, key string) func(*Exporter) error {
    return func(e *Exporter) error {
        e.clientcert = cert
        e.clientkey = key
        return nil
    }
}

var healthzOK = []byte("ok\n")

func (e *Exporter) healthz(w http.ResponseWriter, r *http.Request) {
    w.Write(healthzOK)
}



// creates a wrapper for an http.Handler, where the wrapper changes the endpoint according to the query parameter "pool"
// this is intended to be used in multi-pool setups, by having nginx or apache forward the URL http://endpoint?pool=X to a /status request on fpm pool X
func (e *Exporter) queryHandler(h http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

      pool := r.URL.Query().Get("pool")
      if pool != "" {
        e.endpoint, _ = url.Parse(e.origEndpoint + "/" + pool)
      } else {
            e.endpoint, _ = url.Parse(e.origEndpoint)
      }
      h.ServeHTTP(w, r)
    })
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
    http.Handle("/metrics", e.queryHandler(promhttp.Handler()))
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

