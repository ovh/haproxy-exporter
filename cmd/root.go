package cmd

import (
	"fmt"
	"bytes"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	// "github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"net/http"

	"github.com/prometheus/common/expfmt"
	"stash.ovh.net/metrics/haproxy-exporter/core"
)

var cfgFile string
var verbose bool

// Aggregator init - define command line arguments.
func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file to use")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	RootCmd.Flags().String("listen", "127.0.0.1:9100", "listen address")

	viper.BindPFlags(RootCmd.Flags())
}

// Load config - initialize defaults and read config.
func initConfig() {
	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	// Defaults
	viper.SetDefault("loopDuration", 1000)
	viper.SetDefault("maxConcurrent", 50)

	// Bind environment variables
	viper.SetEnvPrefix("haexport")
	viper.AutomaticEnv()

	// Set config search path
	viper.AddConfigPath("/etc/haproxy-exporter/")
	viper.AddConfigPath("$HOME/.haproxy-exporter")
	viper.AddConfigPath(".")

	// Load default config
	viper.SetConfigName("default")
	if err := viper.MergeInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Debug("No default config file found")
		} else {
			log.Panicf("Fatal error in default config file: %v \n", err)
		}
	}

	// Load user defined config
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		err := viper.ReadInConfig()
		if err != nil {
			log.Panicf("Fatal error in config file: %v \n", err)
		}
	}
}

type Source struct {
	URI    string
	Labels map[string]interface{}
}

// RootCmd launch the aggregator agent.
var RootCmd = &cobra.Command{
	Use:   "haproxy-exporter",
	Short: "HAProxy exporter expose HAProxy stats as Prometheus metrics",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("HAProxy exporter starting")

		// Custom reporter (drop go* metrics)
		var registry = prometheus.NewRegistry()

		// Load sources
		var sources []Source
		if err := viper.UnmarshalKey("sources", &sources); err != nil {
			log.Fatal("Unable to decode 'sources', %v", err)
		}

		// Build exporters
		var godMode = 200
		exporters := make([]*core.Exporter, len(sources) * godMode)

		for i, _ := range exporters {
			s := sources[i%len(sources)]
			// Setup labels
			labels := make(map[string]string)
			labels["i"] = fmt.Sprintf("%v", i)
			for k, v := range s.Labels {
				labels[k] = fmt.Sprintf("%v", v)
			}

			exporter, err := core.NewExporter(s.URI, 5*time.Second, labels) // FIXME
			if err != nil {
				log.Fatal(err)
			}
			// registry.MustRegister(exporter) FIXME
			exporters[i] = exporter
		}

		// Start beamer
		core.NewBeamer(exporters, registry)

		// Setup http routing
		// http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		// 	ErrorLog:           nil,
		// 	ErrorHandling:      promhttp.ContinueOnError,
		// 	DisableCompression: false,
		// }))
		http.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			mfs, err := registry.Gather()
			if err != nil {
				return
			}

			contentType := expfmt.Negotiate(req.Header)
			var buf bytes.Buffer
			enc := expfmt.NewEncoder(&buf, contentType)
			var lastErr error
			for _, mf := range mfs {
				enc.Encode(mf)
			}
			if lastErr != nil && buf.Len() == 0 {
				http.Error(w, "No metrics encoded, last error:\n\n"+err.Error(), http.StatusInternalServerError)
				return
			}
			header := w.Header()
			header.Set("Content-Type", string(contentType))


			for _, e := range exporters {
				w.Write(e.Metrics().Bytes())
			}
			w.Write(buf.Bytes())

		}))
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<html>
	             <head><title>Haproxy Exporter</title></head>
	             <body>
	             <h1>Haproxy Exporter</h1>
	             <p><a href="/metrics">Metrics</a></p>
	             </body>
	             </html>`))
		})

		log.Info("Started")

		// ticker := time.NewTicker(20000 * time.Millisecond)
		// go func() {
		// 	for {
		// 		select {
		// 		case <-ticker.C:
		// 			start := time.Now()
		// 			registry.Gather()
		// 			log.Infof("%v", time.Since(start))
		// 		}
		// 	}
		// }()

		log.Infof("Listen %s", viper.GetString("listen"))
		log.Fatal(http.ListenAndServe(viper.GetString("listen"), nil))
	},
}
