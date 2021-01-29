package main

import (
	"go.uber.org/zap"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	exporter "github.com/bakins/php-fpm-exporter"
)

func main() {
	var (
		addr            = kingpin.Flag("addr", "listen address for metrics handler").Default("127.0.0.1:8080").Envar("LISTEN_ADDR").String()
		endpoint        = kingpin.Flag("endpoint", "url for php-fpm status").Default("http://127.0.0.1:9000/status").Envar("ENDPOINT_URL").String()
		fcgiEndpoint    = kingpin.Flag("fastcgi", "fastcgi url. If this is set, fastcgi will be used instead of HTTP").Envar("FASTCGI_URL").String()
		metricsEndpoint = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics. Cannot be /").Default("/metrics").Envar("TELEMETRY_PATH").String()
		statusTimeout   = kingpin.Flag("status.timeout", "Scrape timeout for php-fpm status. If unset, then will wait forever.").Envar("STATUS_TIMEOUT").Duration()
	)

	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger, err := exporter.NewLogger()
	if err != nil {
		panic(err)
	}

	e, err := exporter.New(
		exporter.SetAddress(*addr),
		exporter.SetEndpoint(*endpoint),
		exporter.SetFastcgi(*fcgiEndpoint),
		exporter.SetLogger(logger),
		exporter.SetMetricsEndpoint(*metricsEndpoint),
		exporter.SetStatusTimeout(statusTimeout),
	)

	if err != nil {
		logger.Fatal("failed to create exporter", zap.Error(err))
	}

	if err := e.Run(); err != nil {
		logger.Fatal("failed to run exporter", zap.Error(err))
	}
}
