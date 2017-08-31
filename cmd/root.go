package cmd

import (
	"fmt"
	"net/http"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ovh/haproxy-exporter/core"
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
	viper.SetDefault("scanDuration", 1000)
	viper.SetDefault("scrapeTimeout", 5000)
	viper.SetDefault("maxConcurrent", 50)
	viper.SetDefault("flushPeriod", 10000)

	// Bind environment variables
	viper.SetEnvPrefix("haexport")
	viper.AutomaticEnv()

	// Set config search path
	viper.AddConfigPath("/etc/haproxy-exporter/")
	viper.AddConfigPath("$HOME/.haproxy-exporter")
	viper.AddConfigPath(".")

	// Load config
	viper.SetConfigName("config")
	if err := viper.MergeInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Debug("No config file found")
		} else {
			log.Panicf("Fatal error in config file: %v \n", err)
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

// Source defined a HAProxy stats source
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

		// Load sources
		var sources []Source
		if err := viper.UnmarshalKey("sources", &sources); err != nil {
			log.Fatalf("Unable to decode 'sources', %v", err)
		}

		log.Info(sources)
		if len(sources) == 0 {
			log.Fatal("No sources defined, dying")
		}

		// Build exporters
		exporters := make([]*core.Exporter, len(sources))

		for i := range exporters {
			s := sources[i]
			// Setup labels
			labels := make(map[string]string)
			for k, v := range s.Labels {
				labels[k] = fmt.Sprintf("%v", v)
			}
			for k, v := range viper.GetStringMapString("labels") {
				labels[k] = fmt.Sprintf("%v", v)
			}

			exporter, err := core.NewExporter(s.URI,
				time.Duration(viper.GetInt("scrapeTimeout"))*time.Millisecond,
				labels,
				viper.GetStringSlice("metrics"))
			if err != nil {
				log.Fatal(err)
			}
			exporters[i] = exporter
		}
		log.Infof("Exporters started - %v", len(exporters))

		// Start beamer
		b := core.NewBeamer(exporters, viper.GetStringMapString("labels"))
		log.Infof("Beamer started")

		// Setup http
		http.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Write(b.Metrics().Bytes())
			for _, e := range exporters {
				w.Write(e.Metrics().Bytes())
			}
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
		log.Infof("Http started")

		if viper.IsSet("flushPath") {
			flushPath := viper.GetString("flushPath")
			ticker := time.NewTicker(time.Duration(viper.GetInt("flushPeriod")) * time.Millisecond)
			go func() {
				for {
					select {
					case <-ticker.C:
						// Write to tmp file
						path := fmt.Sprintf("%v%v", flushPath, time.Now().Unix())
						log.Debugf("Flush to file: %v%v", path, ".tmp")
						file, err := os.Create(path + ".tmp")
						if err != nil {
							log.Errorf("Flush failed: %v", err)
						}

						file.Write(b.Metrics().Bytes())
						for _, e := range exporters {
							file.Write(e.Metrics().Bytes())
						}

						file.Close()

						// Move tmp file to metrics one
						log.Debugf("Move to file: %v%v", path, ".metrics")
						os.Rename(path+".tmp", path+".metrics")
					}
				}
			}()
			log.Infof("Flush routine started")
		}

		log.Info("Started")

		log.Infof("Listen %s", viper.GetString("listen"))
		log.Fatal(http.ListenAndServe(viper.GetString("listen"), nil))
	},
}
