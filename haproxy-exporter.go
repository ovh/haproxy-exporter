// HAProxy exporter expose HAProxy stats as Prometheus metrics.
//
// Usage
//
// 		haproxy-exporter  [flags]
// Flags:
//       --config string   config file to use
//       --help            display help
//   -v, --verbose         verbose output
package main

import (
	log "github.com/sirupsen/logrus"

	"github.com/ovh/haproxy-exporter/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		log.Panicf("%v", err)
	}
}
