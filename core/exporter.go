package core

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"strconv"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// TODO implements gather interface

func newHAMetric(metricName string, docString string, constLabels prometheus.Labels) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "haproxy",
			Subsystem:   "stats",
			Name:        metricName,
			Help:        docString,
			ConstLabels: constLabels,
		},
		[]string{"pxname", "svname", "type"},
	)
}

// Exporter collects HAProxy stats from the given URI and exports them using
// the prometheus metrics package.
type Exporter struct {
	URI   string
	mutex sync.RWMutex
	fetch func() (io.ReadCloser, error)

	metrics  map[int]*prometheus.GaugeVec
	registry *prometheus.Registry
	prometheus bytes.Buffer
}

// NewExporter returns an initialized Exporter.
func NewExporter(uri string, timeout time.Duration, labels prometheus.Labels) (*Exporter, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	var fetch func() (io.ReadCloser, error)
	switch u.Scheme {
	case "http", "https", "file":
		fetch = fetchHTTP(uri, timeout)
	case "unix":
		fetch = fetchUnix(u, timeout)
	default:
		return nil, fmt.Errorf("unsupported scheme: %q", u.Scheme)
	}

	e := &Exporter{
		URI:      uri,
		fetch:    fetch,
		registry: prometheus.NewRegistry(),
		metrics: map[int]*prometheus.GaugeVec{
			// pxname
			// svname
			2:  newHAMetric("qcur", "Current number of queued requests not assigned to any server.", labels),
			3:  newHAMetric("qmax", "Maximum observed number of queued requests not assigned to any server.", labels),
			4:  newHAMetric("scur", "Current number of active sessions.", labels),
			5:  newHAMetric("smax", "Maximum observed number of active sessions.", labels),
			6:  newHAMetric("slim", "Configured session limit.", labels),
			7:  newHAMetric("stot", "Total number of connections.", labels),
			8:  newHAMetric("bin", "Current total of incoming bytes.", labels),
			9:  newHAMetric("bout", "Current total of outgoing bytes.", labels),
			10: newHAMetric("dreq", "Total of requests denied for security.", labels),
			11: newHAMetric("dresp", "Total of responses denied for security.", labels),
			12: newHAMetric("ereq", "Total of request errors.", labels),
			13: newHAMetric("econ", "Total of connection errors.", labels),
			14: newHAMetric("eresp", "Total of response errors.", labels),
			15: newHAMetric("wretr", "Total of retry warnings.", labels),
			16: newHAMetric("wredis", "Total of redispatch warnings.", labels),
			17: newHAMetric("status", "Current health status of the backend (1 = UP, 0 = DOWN).", labels),
			18: newHAMetric("weight", "Total weight of the servers in the backend.", labels),
			19: newHAMetric("act", "Number of active servers (backend), server is active (server).", labels),
			20: newHAMetric("bck", "Number of active servers (backend), server is active (server).", labels),
			21: newHAMetric("chkfail", "Total number of failed health checks.", labels),
			22: newHAMetric("chkdown", "The backend counter counts transitions to the whole backend being down, rather than the sum of the counters for each server.", labels),
			23: newHAMetric("lastchg", "Number of seconds since the last UP<->DOWN transition", labels),
			24: newHAMetric("downtime", "Total downtime in seconds.", labels),
			25: newHAMetric("qlimit", "Configured maxqueue for the server.", labels),
			26: newHAMetric("pid", "Process id.", labels),
			27: newHAMetric("iid", "Unique proxy id.", labels),
			28: newHAMetric("sid", "Server id.", labels),
			29: newHAMetric("throttle", "Current throttle percentage for the server.", labels),
			30: newHAMetric("lbtot", "Total number of times a server was selected.", labels),
			31: newHAMetric("tracked", "Id of proxy/server if tracking is enabled.", labels),
			// type
			33: newHAMetric("current_session_rate", "Current number of sessions per second over last elapsed second.", labels),
			34: newHAMetric("limit_session_rate", "Configured limit on new sessions per second.", labels),
			35: newHAMetric("max_session_rate", "Maximum observed number of sessions per second.", labels),
			// check_status
			// check_code
			38: newHAMetric("check_duration", "Previously run health check duration, in milliseconds", labels),
			39: newHAMetric("hrsp_1xx", "Http responses with 1xx code.", labels),
			40: newHAMetric("hrsp_2xx", "Http responses with 2xx code.", labels),
			41: newHAMetric("hrsp_3xx", "Http responses with 3xx code.", labels),
			42: newHAMetric("hrsp_4xx", "Http responses with 4xx code.", labels),
			43: newHAMetric("hrsp_5xx", "Http responses with 5xx code.", labels),
			44: newHAMetric("hrsp_other", "Http responses with other code.", labels),
			// hanafail
			46: newHAMetric("req_rate", "HTTP requests per second over last elapsed second.", labels),
			47: newHAMetric("req_rate_max", "Max number of HTTP requests per second observed.", labels),
			48: newHAMetric("req_tot", "Total HTTP requests received.", labels),
			49: newHAMetric("cli_abrt", "Number of data transfers aborted by the client.", labels),
			50: newHAMetric("srv_abrt", "Number of data transfers aborted by the server.", labels),
			51: newHAMetric("comp_in", "Number of HTTP response bytes fed to the compressor.", labels),
			52: newHAMetric("comp_out", "Number of HTTP response bytes emitted by the compressor.", labels),
			53: newHAMetric("comp_byp", "Number of bytes that bypassed the HTTP compressor.", labels),

			54: newHAMetric("comp_rsp", "Number of HTTP responses that were compressed.", labels),
			55: newHAMetric("lastsess", "Number of seconds since last session assigned to server/backend.", labels),
			// last_chk
			// last_agt
			58: newHAMetric("qtime", "Average queue time in ms over the 1024 last requests.", labels),
			59: newHAMetric("ctime", "Average connect time in ms over the 1024 last requests.", labels),
			60: newHAMetric("rtime", "Average response time in ms over the 1024 last requests.", labels),
			61: newHAMetric("ttime", "Average total session time in ms over the 1024 last requests.", labels),
		},
	}

	e.registry.MustRegister(e)

	return e, nil
}

// Describe describes all the metrics ever exported by the HAProxy exporter.
// It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range e.metrics {
		m.Describe(ch)
	}
}

// Collect delivers HAProxy stats as Prometheus metrics.
// It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	for _, m := range e.metrics {
		m.Collect(ch)
	}
}

// Metrics delivers HAProxy stats as Prometheus metrics.
func (e *Exporter) Metrics() *bytes.Buffer {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return bytes.NewBuffer(e.prometheus.Bytes())
}

func fetchHTTP(uri string, timeout time.Duration) func() (io.ReadCloser, error) {
	client := http.Client{
		Timeout: timeout,
	}

	return func() (io.ReadCloser, error) {
		resp, err := client.Get(uri)
		if err != nil {
			return nil, err
		}
		if !(resp.StatusCode >= 200 && resp.StatusCode < 300) {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func fetchUnix(u *url.URL, timeout time.Duration) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		f, err := net.DialTimeout("unix", u.Path, timeout)
		if err != nil {
			return nil, err
		}
		if err := f.SetDeadline(time.Now().Add(timeout)); err != nil {
			f.Close()
			return nil, err
		}
		cmd := "show stat\n"
		n, err := io.WriteString(f, cmd)
		if err != nil {
			f.Close()
			return nil, err
		}
		if n != len(cmd) {
			f.Close()
			return nil, errors.New("write error")
		}
		return f, nil
	}
}

// clear reset all the metrics
func (e *Exporter) clear() {
	// protect consistency
	e.mutex.Lock()
	defer e.mutex.Unlock()
	for _, m := range e.metrics {
		m.Reset()
	}
}

// Scrape retrive HAProxy data
func (e *Exporter) Scrape() int {
	parseFailures := 0

	body, err := e.fetch()

	// Delete previous metrics
	e.clear()

	if err != nil {
		return -1
	}
	defer body.Close()

	// protect consistency
	e.mutex.Lock()
	defer e.mutex.Unlock()

	reader := csv.NewReader(body)
	reader.TrailingComma = true
	reader.Comment = '#'

loop:
	for {
		row, err := reader.Read()
		if err != nil {
			switch err {
			case io.EOF:
				break loop
			default:
				if _, ok := err.(*csv.ParseError); ok {
					parseFailures++
					continue loop
				}
				return -1
			}
		}

		if len(row) == 0 {
			continue
		}

		// Discard too short row
		const minCsvFieldCount = 52
		if len(row) < minCsvFieldCount {
			log.Warnf("Wrong CSV field count: %d < %d", len(row), minCsvFieldCount)
			parseFailures++
			continue
		}

		// Get pxname, svname and type
		pxname, svname, type_ := row[0], row[1], row[32]

		parseFailures += e.exportCsvFields(e.metrics, row, pxname, svname, type_)
	}

	// Gather
	go func() {

		mfs, err := e.registry.Gather()
		if err != nil {
			log.Errorf("error gathering metrics: %v", err)
		}

		// exports metrics
		e.mutex.Lock()
		defer e.mutex.Unlock()
		e.prometheus.Reset()
		enc := expfmt.NewEncoder(&e.prometheus, expfmt.FmtText)
		for _, mf := range mfs {
			if err := enc.Encode(mf); err != nil {
				log.Errorf("error encoding metric family: %v", err)
			}
		}
	}()

	return parseFailures
}

func (e *Exporter) exportCsvFields(metrics map[int]*prometheus.GaugeVec, csvRow []string, labels ...string) int {
	parseFailures := 0

	for fieldIdx, metric := range metrics {
		valueStr := csvRow[fieldIdx]
		if valueStr == "" {
			continue
		}

		var value int64
		if fieldIdx == 17 { // status field
			switch valueStr {
			case "UP", "UP 1/3", "UP 2/3", "OPEN", "no check":
				value = 1
			case "DOWN", "DOWN 1/2", "NOLB", "MAINT":
				value = 0
			}
			value = 0
		} else {
			var err error
			value, err = strconv.ParseInt(valueStr, 10, 64)
			if err != nil {
				log.Warnf("Can't parse CSV field value %s: %v", valueStr, err)
				parseFailures++
				continue
			}
		}

		metric.WithLabelValues(labels...).Set(float64(value))
	}

	return parseFailures
}

// func (e *Exporter) URI() string {
//   return e.URI
// }
