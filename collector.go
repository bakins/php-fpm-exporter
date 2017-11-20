package exporter

import (

    "crypto/tls"

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
    phpProcesses       *prometheus.Desc
    maxActiveProcesses *prometheus.Desc
    maxChildrenReached *prometheus.Desc
    slowRequests       *prometheus.Desc
    scrapeFailures     *prometheus.Desc
    failureCount       int
}

const metricsNamespace = "phpfpm"

func newFuncMetric(metricName string, docString string, labels []string) *prometheus.Desc {
    return prometheus.NewDesc(
        prometheus.BuildFQName(metricsNamespace, "", metricName),
        docString, labels, nil,
    )
}

func (e *Exporter) newCollector() *collector {
    return &collector{
        exporter:           e,
        up:                 newFuncMetric("up", "able to contact php-fpm", nil),
        acceptedConn:       newFuncMetric("accepted_connections_total", "Total number of accepted connections", nil),
        listenQueue:        newFuncMetric("listen_queue_connections", "Number of connections that have been initiated but not yet accepted", nil),
        maxListenQueue:     newFuncMetric("listen_queue_max_connections", "Max number of connections the listen queue has reached since FPM start", nil),
        listenQueueLength:  newFuncMetric("listen_queue_length_connections", "The length of the socket queue, dictating maximum number of pending connections", nil),
        phpProcesses:       newFuncMetric("processes_total", "process count", []string{"state"}),
        maxActiveProcesses: newFuncMetric("active_max_processes", "Maximum active process count", nil),
        maxChildrenReached: newFuncMetric("max_children_reached_total", "Number of times the process limit has been reached", nil),
        slowRequests:       newFuncMetric("slow_requests_total", "Number of requests that exceed request_slowlog_timeout", nil),
        scrapeFailures:     newFuncMetric("scrape_failures_total", "Number of errors while scraping php_fpm", nil),
    }
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

func (c *collector) getData(u *url.URL) ([]byte, error) {
    req := http.Request{
        Method:     "GET",
        URL:        u,
        Proto:      "HTTP/1.1",
        ProtoMajor: 1,
        ProtoMinor: 1,
        Header:     make(http.Header),
        Host:       u.Host,
    }


    var client *http.Client
/*
  ClientCert string `yaml:"clientcert"`
    ClientKey string `yaml:"clientkey"`
    AuthUser string `yaml:"authuser"`
    AuthPass string `yaml:"authpass"`
    */

    if (c.exporter.httpconf.ClientCert != "") {
        cert, err := tls.LoadX509KeyPair(c.exporter.httpconf.ClientCert, c.exporter.httpconf.ClientKey)
        if err != nil {
            return nil, errors.Wrap(err, "Could not load cert and/or key")
        }

        client = &http.Client{
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{
                    Certificates: []tls.Certificate{cert},
                    InsecureSkipVerify: false,
                },
            },
        }
    } else {
        client = http.DefaultClient
    }

    if (c.exporter.httpconf.AuthUser != "") {
        req.SetBasicAuth(c.exporter.httpconf.AuthUser, c.exporter.httpconf.AuthPass)
    }


    resp, err := client.Do(&req)
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
    body, err := c.getData(c.exporter.endpoint)
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
        labels := []string{}

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
            desc = c.phpProcesses
            valueType = prometheus.GaugeValue
            labels = append(labels, "idle")
        case "active processes":
            desc = c.phpProcesses
            valueType = prometheus.GaugeValue
            labels = append(labels, "active")
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

        m, err := prometheus.NewConstMetric(desc, valueType, float64(value), labels...)
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

