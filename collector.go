package exporter

import (
	"container/list"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	fcgiclient "github.com/tomasen/fcgi_client"
	"go.uber.org/zap"
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
	phpProcesses       *prometheus.Desc
	maxActiveProcesses *prometheus.Desc
	maxChildrenReached *prometheus.Desc
	slowRequests       *prometheus.Desc
	scrapeFailures     *prometheus.Desc
	targets            []*targetCollector
}

const metricsNamespace = "phpfpm"

func newFuncMetric(metricName string, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName(metricsNamespace, "", metricName),
		docString, labels, nil,
	)
}

func (e *Exporter) newCollector() *collector {
	c := &collector{
		exporter:           e,
		up:                 newFuncMetric("up", "able to contact php-fpm", []string{"target"}),
		acceptedConn:       newFuncMetric("accepted_connections_total", "Total number of accepted connections", []string{"target"}),
		listenQueue:        newFuncMetric("listen_queue_connections", "Number of connections that have been initiated but not yet accepted", []string{"target"}),
		maxListenQueue:     newFuncMetric("listen_queue_max_connections", "Max number of connections the listen queue has reached since FPM start", []string{"target"}),
		listenQueueLength:  newFuncMetric("listen_queue_length_connections", "The length of the socket queue, dictating maximum number of pending connections", []string{"target"}),
		phpProcesses:       newFuncMetric("processes_total", "process count", []string{"target", "state"}),
		maxActiveProcesses: newFuncMetric("active_max_processes", "Maximum active process count", []string{"target"}),
		maxChildrenReached: newFuncMetric("max_children_reached_total", "Number of times the process limit has been reached", []string{"target"}),
		slowRequests:       newFuncMetric("slow_requests_total", "Number of requests that exceed request_slowlog_timeout", []string{"target"}),
		scrapeFailures:     newFuncMetric("scrape_failures_total", "Number of errors while scraping php_fpm", []string{"target"}),
	}

	for k, v := range e.targets {
		c.targets = append(c.targets, newTargetCollector(k, v, c))
	}
	return c
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.scrapeFailures
	ch <- c.acceptedConn
	ch <- c.listenQueue
	ch <- c.maxListenQueue
	ch <- c.listenQueueLength
	ch <- c.phpProcesses
	ch <- c.maxActiveProcesses
	ch <- c.maxChildrenReached
	ch <- c.slowRequests
}

func getDataFastcgi(u *url.URL) ([]byte, error) {
	path := u.Path
	host := u.Host

	if path == "" || u.Scheme == "unix" {
		path = "/status"
	}
	if u.Scheme == "unix" {
		host = u.Path
	}

	env := map[string]string{
		"SCRIPT_FILENAME": path,
		"SCRIPT_NAME":     path,
	}

	fcgi, err := fcgiclient.Dial(u.Scheme, host)
	if err != nil {
		return nil, errors.Wrap(err, "fastcgi dial failed")
	}

	defer fcgi.Close()

	resp, err := fcgi.Get(env)
	if err != nil {
		return nil, errors.Wrap(err, "fastcgi get failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 0 {
		return nil, errors.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read fastcgi body")
	}

	return body, nil
}

func getDataHTTP(u *url.URL) ([]byte, error) {
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
	var wg sync.WaitGroup

	// simple "queue" of metrics
	metrics := list.New()
	var mutex sync.Mutex

	for _, t := range c.targets {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := t.collect()
			if err != nil {
				c.exporter.logger.Error("error collecting php-fpm metrics", zap.String("target", t.name), zap.Error(err))
			}
			mutex.Lock()
			defer mutex.Unlock()

			for _, m := range out {
				metrics.PushBack(m)
			}
		}()
	}

	wg.Wait()

	// should be no writers to the list, but just in case
	mutex.Lock()
	defer mutex.Unlock()

	for e := metrics.Front(); e != nil; e = e.Next() {
		m := e.Value.(prometheus.Metric)
		ch <- m
	}
}

type targetCollector struct {
	name      string
	url       *url.URL
	failures  int
	up        bool
	collector *collector
}

func newTargetCollector(name string, url *url.URL, collector *collector) *targetCollector {
	return &targetCollector{
		name:      name,
		url:       url,
		collector: collector,
	}
}

func (t *targetCollector) collect() ([]prometheus.Metric, error) {
	up := 1.0
	var (
		body []byte
		err  error
		out  []prometheus.Metric
	)

	switch t.url.Scheme {
	case "http", "https":
		body, err = getDataHTTP(t.url)
	default:
		body, err = getDataFastcgi(t.url)
	}

	if err != nil {
		up = 0.0
		t.failures++
	}

	out = append(out,
		prometheus.MustNewConstMetric(
			t.collector.up,
			prometheus.GaugeValue,
			up,
			t.name,
		))

	out = append(out,
		prometheus.MustNewConstMetric(
			t.collector.scrapeFailures,
			prometheus.CounterValue,
			float64(t.failures),
			t.name,
		))

	if err != nil {
		return out, err
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
		labels := []string{t.name}

		switch key {
		case "accepted conn":
			desc = t.collector.acceptedConn
			valueType = prometheus.CounterValue
		case "listen queue":
			desc = t.collector.listenQueue
			valueType = prometheus.GaugeValue
		case "max listen queue":
			desc = t.collector.maxListenQueue
			valueType = prometheus.CounterValue
		case "listen queue len":
			desc = t.collector.listenQueueLength
			valueType = prometheus.GaugeValue
		case "idle processes":
			desc = t.collector.phpProcesses
			valueType = prometheus.GaugeValue
			labels = append(labels, "idle")
		case "active processes":
			desc = t.collector.phpProcesses
			valueType = prometheus.GaugeValue
			labels = append(labels, "active")
		case "max active processes":
			desc = t.collector.maxActiveProcesses
			valueType = prometheus.CounterValue
		case "max children reached":
			desc = t.collector.maxChildrenReached
			valueType = prometheus.CounterValue
		case "slow requests":
			desc = t.collector.slowRequests
			valueType = prometheus.CounterValue
		default:
			continue
		}

		out = append(out,
			prometheus.MustNewConstMetric(desc, valueType, float64(value), labels...))
	}

	return out, nil
}
