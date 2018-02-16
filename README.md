php-fpm-exporter
================

Export php-fpm metrics in [Prometheus](https://prometheus.io/) format.

See [Releases](https://github.com/bakins/php-fpm-exporter/releases) for pre-built binaries.

Also availible at [quay.io/bakins/php-fpm-exporter](https://quay.io/repository/bakins/php-fpm-exporter)

Build
=====

Requires [Go](https://golang.org/doc/install). Tested with Go 1.8+.

Clone this repo into your `GOPATH` (`$HOME/go` by default) and run build:

```
mkdir -p $HOME/go/src/github.com/bakins
cd $HOME/go/src/github.com/bakins
git clone https://github.com/bakins/php-fpm-exporter
cd php-fpm-exporter
./script/build
```

You should then have two executables: php-fpm-exporter.linux.amd64 and php-fpm-exporter.darwin.amd64

You may want to rename for your local OS, ie `mv php-fpm-exporter.darwin.amd64 php-fpm-exporter`

Running
=======

```
./php-fpm-exporter --help
php-fpm metrics exporter

Usage:
  php-fpm-exporter [flags]

Flags:
      --addr string       listen address for metrics handler (default "127.0.0.1:8080")
      --endpoint string   url for php-fpm status (default "http://127.0.0.1:9000/status")
      --fastcgi string    fastcgi url. If this is set, fastcgi will be used instead of HTTP
```

When running, a simple healthcheck is available on `/healthz`

To use the HTTP endpoint you must pass through `/status` in your webserver 
and configure php-fpm to handle status requests. Example for nginx: https://easyengine.io/tutorials/php/fpm-status-page/

To use Fastcgi, set `--fastcgi` to a url such as `tcp://127.0.0.1:9090/status` if php-fpm is listening on a tcp socket or 
`unix:///path/to/php.sock` for a unix socket. Note: php-fpm must be configured to use `/status` if using a unix socket, `php-fpm-exporter` does not currently support changing this.

Metrics
=======

Metrics will be exposes on `/metrics`

LICENSE
========

See [LICENSE](./LICENSE)

loosely based on https://github.com/peakgames/php-fpm-prometheus/ which is MIT.
