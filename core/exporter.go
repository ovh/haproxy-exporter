package core

import (
	"bytes"
	// "encoding/csv"
	"errors"
	"fmt"
	"github.com/gwenn/yacr"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

const (
	rowLength = 62
)

// Exporter collects HAProxy stats from the given URI and exports them as
// warp10 metrics package.
type Exporter struct {
	URI   string
	mutex sync.RWMutex
	fetch func() (io.ReadCloser, error)

	metrics   map[int]string
	sensision bytes.Buffer
	labels    string
}

// NewExporter returns an initialized Exporter.
func NewExporter(uri string, timeout time.Duration, labels map[string]string, metrics []string) (*Exporter, error) {
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
		URI:   uri,
		fetch: fetch,
		metrics: map[int]string{
			// pxname
			// svname
			2:  "qcur",
			3:  "qmax",
			4:  "scur",
			5:  "smax",
			6:  "slim",
			7:  "stot",
			8:  "bin",
			9:  "bout",
			10: "dreq",
			11: "dresp",
			12: "ereq",
			13: "econ",
			14: "eresp",
			15: "wretr",
			16: "wredis",
			17: "status",
			18: "weight",
			19: "act",
			20: "bck",
			21: "chkfail",
			22: "chkdown",
			23: "lastchg",
			24: "downtime",
			25: "qlimit",
			26: "pid",
			27: "iid",
			28: "sid",
			29: "throttle",
			30: "lbtot",
			31: "tracked",
			// type
			33: "current_session_rate",
			34: "limit_session_rate",
			35: "max_session_rate",
			36: "check_status",
			37: "check_code",
			38: "check_duration",
			39: "hrsp_1xx",
			40: "hrsp_2xx",
			41: "hrsp_3xx",
			42: "hrsp_4xx",
			43: "hrsp_5xx",
			44: "hrsp_other",
			// hanafail
			46: "req_rate",
			47: "req_rate_max",
			48: "req_tot",
			49: "cli_abrt",
			50: "srv_abrt",
			51: "comp_in",
			52: "comp_out",
			53: "comp_byp",

			54: "comp_rsp",
			55: "lastsess",
			56: "last_chk",
			// last_agt
			58: "qtime",
			59: "ctime",
			60: "rtime",
			61: "ttime",
		},
	}

	// filter
	if len(metrics) > 0 {
		for i := range e.metrics {
			found := false
			for m := range metrics {
				if e.metrics[i] == metrics[m] {

					found = true
					break
				}
			}

			if !found {
				delete(e.metrics, i)
			}
		}
	}

	for k := range labels {
		e.labels += k + "=" + labels[k] + ","
	}

	return e, nil
}

// Metrics delivers HAProxy stats as warp10 metrics.
func (e *Exporter) Metrics() *bytes.Buffer {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return bytes.NewBuffer(e.sensision.Bytes())
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
			log.Debug(resp.Body)
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
	e.sensision.Reset()
}

// Scrape retrive HAProxy data
func (e *Exporter) Scrape() bool {
	body, err := e.fetch()

	// Delete previous metrics
	e.clear()

	if err != nil {
		return false
	}
	defer body.Close()

	now := fmt.Sprintf("%v// haproxy.stats.", time.Now().UnixNano()/1000)

	// protect consistency
	e.mutex.Lock()
	defer e.mutex.Unlock()

	r := yacr.DefaultReader(body)
	r.SkipRecords(1) // first line is comment

	// Build sparse value array
	values := make([]*string, rowLength)
	for i := 0; i < rowLength; i++ {
		if e.metrics[i] == "" && i != 0 && i != 1 && i != 32 {
			continue
		}
		var s string
		values[i] = &s
	}

	var i = 0
	for r.Scan() {
		if i < rowLength && values[i] != nil {
			r.Value(values[i])
		}

		if r.EndOfRecord() {
			i = 0
			for fieldIdx := range e.metrics {
				valueStr := values[fieldIdx]
				if *valueStr == "" {
					continue
				}

				value := *valueStr
				if fieldIdx == 17 { // status field
					switch *valueStr {
					case "UP", "UP 1/3", "UP 2/3", "OPEN", "no check":
						value = "1"
					case "DOWN", "DOWN 1/2", "NOLB", "MAINT":
						value = "0"
					default:
						value = "0"
					}
				}

				t := ""
				switch *values[32] {
				case "0":
					t = "frontend"
				case "1":
					t = "backend"
				case "2":
					t = "server"
				case "3":
					t = "listen"
				}

				gts := now + e.metrics[fieldIdx] + "{" + e.labels + "pxname=" + *values[0] + ",svname=" + *values[1] + ",type=" + t + "} " + value + "\n"
				e.sensision.WriteString(gts)
			}
		} else {
			i++
		}
	}
	if err := r.Err(); err != nil {
		fmt.Println(err)
	}

	return true
}
