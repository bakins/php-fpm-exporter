package exporter

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"go.uber.org/zap"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	statusLineRegexp = regexp.MustCompile(`(?m)^(.*):\s+(.*)$`)
)

type collector struct {
	exporter           *Exporter
	up                 *prometheus.Desc
	acceptedConn       *prometheus.Desc
	listenQueue        *prometheus.Desc
	maxListenQueue     *prometheus.Desc
	listenQueueLength  *prometheus.Desc
	idleProcesses      *prometheus.Desc
	activeProcesses    *prometheus.Desc
	totalProcesses     *prometheus.Desc
	maxActiveProcesses *prometheus.Desc
	maxChildrenReached *prometheus.Desc
	slowRequests       *prometheus.Desc
	scrapeFailures     *prometheus.Desc
	failureCount       int
}

const metricsNamespace = "phpfpm"

func newFuncMetric(metricName string, docString string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName(metricsNamespace, "", metricName),
		docString, nil, nil,
	)
}

func (e *Exporter) newCollector() *collector {
	return &collector{
		exporter:           e,
		up:                 newFuncMetric("up", "able to contact php-fpm"),
		acceptedConn:       newFuncMetric("accepted_connections_total", "Total number of accepted connections"),
		listenQueue:        newFuncMetric("listen_queue_connections", "Number of connections that have been initiated but not yet accepted"),
		maxListenQueue:     newFuncMetric("listen_queue_max_connections", "Max number of connections the listen queue has reached since FPM start"),
		listenQueueLength:  newFuncMetric("listen_queue_length_connections", "The length of the socket queue, dictating maximum number of pending connections"),
		idleProcesses:      newFuncMetric("idle_processes", "Idle process count"),
		activeProcesses:    newFuncMetric("active_processes", "Active process count"),
		totalProcesses:     newFuncMetric("total_processes", "Total process count"),
		maxActiveProcesses: newFuncMetric("active_max_processes", "Maximum active process count"),
		maxChildrenReached: newFuncMetric("max_children_reached_total", "Number of times the process limit has been reached"),
		slowRequests:       newFuncMetric("slow_requests", "Number of requests that exceed request_slowlog_timeout"),
		scrapeFailures:     newFuncMetric("scrape_failures", "Number of errors while scraping php_fpm"),
	}
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.scrapeFailures
	ch <- c.acceptedConn
	ch <- c.listenQueue
	ch <- c.maxListenQueue
	ch <- c.listenQueueLength
	ch <- c.idleProcesses
	ch <- c.activeProcesses
	ch <- c.totalProcesses
	ch <- c.maxActiveProcesses
	ch <- c.maxChildrenReached
	ch <- c.slowRequests
}

func getData(u *url.URL) ([]byte, error) {
	req := http.Request{
		Method:     "GET",
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Host:       u.Host,
	}

	resp, err := http.DefaultClient.Do(&req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read http body")
	}

	return body, nil
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	up := 1.0
	body, err := getData(c.exporter.endpoint)
	if err != nil {
		up = 0.0
		c.exporter.logger.Error("failed to get php-fpm status", zap.Error(err))
		c.failureCount++
	}
	ch <- prometheus.MustNewConstMetric(
		c.up,
		prometheus.GaugeValue,
		up,
	)

	ch <- prometheus.MustNewConstMetric(
		c.scrapeFailures,
		prometheus.CounterValue,
		float64(c.failureCount),
	)

	if up == 0.0 {
		return
	}

	matches := statusLineRegexp.FindAllStringSubmatch(string(body), -1)
	for _, match := range matches {
		key := match[1]
		value, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}

		var desc *prometheus.Desc
		var valueType prometheus.ValueType

		switch key {
		case "accepted conn":
			desc = c.acceptedConn
			valueType = prometheus.CounterValue
		case "listen queue":
			desc = c.listenQueue
			valueType = prometheus.GaugeValue
		case "max listen queue":
			desc = c.maxListenQueue
			valueType = prometheus.CounterValue
		case "listen queue len":
			desc = c.listenQueueLength
			valueType = prometheus.GaugeValue
		case "idle processes":
			desc = c.idleProcesses
			valueType = prometheus.GaugeValue
		case "active processes":
			desc = c.activeProcesses
			valueType = prometheus.GaugeValue
		case "total processes":
			desc = c.totalProcesses
			valueType = prometheus.GaugeValue
		case "max active processes":
			desc = c.maxActiveProcesses
			valueType = prometheus.CounterValue
		case "max children reached":
			desc = c.maxChildrenReached
			valueType = prometheus.CounterValue
		case "slow requests":
			desc = c.slowRequests
			valueType = prometheus.CounterValue
		default:
			continue
		}

		m, err := prometheus.NewConstMetric(desc, valueType, float64(value))
		if err != nil {
			c.exporter.logger.Error(
				"failed to create metrics",
				zap.String("key", key),
				zap.Error(err),
			)
			continue
		}

		ch <- m
	}
}

/*
func (c *collector) collectThreads(ch chan<- prometheus.Metric) {
	t, err := c.driveshaft.getThreads()
	if err != nil {
		c.exporter.logger.Error("failed to get driveshaft status", zap.Error(err))
		ch <- prometheus.MustNewConstMetric(
			c.up,
			prometheus.GaugeValue,
			float64(0),
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		c.up,
		prometheus.GaugeValue,
		float64(1),
	)

	for _, v := range t {
		ch <- prometheus.MustNewConstMetric(
			c.threadsGauge,
			prometheus.GaugeValue,
			float64(v.count),
			v.function, v.state)

	}
}

*/
