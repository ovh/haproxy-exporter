package core

import (
	"io"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
)

// Beamer support HAProxy stats collection
type Beamer struct {
	URI   string
	mutex sync.RWMutex
	fetch func() (io.ReadCloser, error)

	up                             prometheus.Gauge
	totalScrapes, csvParseFailures prometheus.Counter
	metrics                        map[int]*prometheus.GaugeVec
}

func NewBeamer(exporters []*Exporter, registry *prometheus.Registry) {
	duration := viper.GetInt("loopDuration")
	ticker := time.NewTicker(time.Duration(duration/len(exporters)) * time.Millisecond)
  running := make(chan struct{}, viper.GetInt("maxConcurrent"))
	quickStart := make(chan struct{})
	i := 0

	scrapeCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "haproxy",
		Subsystem: "exporter",
		Name:      "scrape",
		Help:      "Number of HAProxy scrape done.",
	})
	registry.MustRegister(scrapeCounter)

	scrapeSkiped := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "haproxy",
		Subsystem: "exporter",
		Name:      "scrape_skiped",
		Help:      "Number of scrape skiped.",
	})
	registry.MustRegister(scrapeSkiped)

	scrapeFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "haproxy",
		Subsystem: "exporter",
		Name:      "scrape_failures",
		Help:      "Number of scrape failures.",
	})
	registry.MustRegister(scrapeFailures)

	parseFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "haproxy",
		Subsystem: "exporter",
		Name:      "parse_failures",
		Help:      "Number of errors while parsing CSV.",
	})
	registry.MustRegister(parseFailures)

	go func() {
		for {
			select {
      case <-quickStart: // FIXME
			case <-ticker.C:
				select {
				case running <- struct{}{}:
					go func() {
						defer func() {
							<-running
						}()
						e := exporters[i]
						res := e.Scrape()
						if res > 0 {
							parseFailures.Add(float64(res))
							log.Warnf("Parse doomed for", e.URI)
						} else if res < 0 {
							scrapeFailures.Inc()
							log.Errorf("Scrape fail for", e.URI)
						}
					}()

					scrapeCounter.Inc()
					i++
					if i >= len(exporters) {
						i = 0
					}
				default:
					scrapeSkiped.Inc()
				}
			}
		}
	}()

  quickStart <- struct{}{}
}
