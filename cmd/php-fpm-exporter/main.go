package main

import (
	"go.uber.org/zap"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	exporter "github.com/bakins/php-fpm-exporter"
)

func main() {
	var (
		addr            = kingpin.Flag("addr", "listen address for metrics handler").Default("127.0.0.1:8080").String()
		endpoint        = kingpin.Flag("endpoint", "url for php-fpm status").Default("http://127.0.0.1:9000/status").String()
		fcgiEndpoint    = kingpin.Flag("fastcgi", "fastcgi url. If this is set, fastcgi will be used instead of HTTP").String()
		metricsEndpoint = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics. Cannot be /").Default("/metrics").String()
		targets         = kingpin.Flag("targets", "targets for scraping in the form name=url").StringMap()
	)

	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger, err := exporter.NewLogger()
	if err != nil {
		panic(err)
	}

	t := *targets
	if len(t) == 0 {
		if *fcgiEndpoint != "" {
			t["default"] = *fcgiEndpoint
		} else {
			t["default"] = *endpoint
		}
	}

	e, err := exporter.New(
		exporter.SetAddress(*addr),
		exporter.SetTargets(t),
		exporter.SetLogger(logger),
		exporter.SetMetricsEndpoint(*metricsEndpoint),
	)

	if err != nil {
		logger.Fatal("failed to create exporter", zap.Error(err))
	}

	if err := e.Run(); err != nil {
		logger.Fatal("failed to run exporter", zap.Error(err))
	}
}
