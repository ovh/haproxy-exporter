# HAProxy-Exporter - HAProxy stats to Sensision Metrics
[![Build Status](https://travis-ci.org/runabove/haproxy-exporter.svg?branch=master)](https://travis-ci.org/runabove/haproxy-exporter)

HAProxy-Exporter scrapes HAProxy stats and expose them as a Sensision HTTP endpoint.

HAProxy-Exporter features:
 - **Simple**: HAProxy-Exporter fetch stats through HTTP endpoint or Unix socket.
 - **Highly scalable**: HAProxy-Exporter can export stats of thousands HAProxy.
 - **Pluggable**: Export your metrics via [Beamium](https://github.com/runabove/beamium).
 - **Versatile**: HAProxy-Exporter can flush metrics to files.

## Status

HAProxy-Exporter is currently used at [OVH](https://www.ovh.com) to monitor thousands of HAProxy instances.
Do not worry about scalability ;)

## Datapoints

HAProxy-Exporter will automatically collect all metrics exposed by HAProxy on its
control socket, including all frontends, bachends and individual servers. To help
classify these metrics, HAProxy-Exporter will name is ``haproxy.stats.[METRIC_NAME]`` and
automatically insert a timestamp with a resolution of a micro-second as well as 3 labels:

- *``pxname``*: Proxy name. This is the string after ``frontend``, ``backend`` and ``listen`` keywords in you HAProxy configuration.
- *``svname``*: Service name. This 'BACKEND', 'FRONTEND' or the name of the ``server`` field for server metrics.
- *``type``*: Type of the object. (0=frontend, 1=backend, 2=server, 3=socket/listener)

Additional labels may be configured in the documentation. See below.

*Sample metrics*
```
1483065169580915// haproxy.stats.eresp{pxname=80,svname=BACKEND,type=1} 41297
1483065169580915// haproxy.stats.cli_abrt{pxname=80,svname=BACKEND,type=1} 1251859
1483065169580915// haproxy.stats.bout{pxname=80,svname=BACKEND,type=1} 553018374845
1483065169580915// haproxy.stats.scur{pxname=80,svname=BACKEND,type=1} 764
1483065169580915// haproxy.stats.dreq{pxname=80,svname=BACKEND,type=1} 0
```

For more informations, please see HaProxy Managment Guide: http://cbonte.github.io/haproxy-dconv/1.7/management.html#9.1

## Building

HAProxy-Exporter is pretty easy to build.
 - Clone the repository
 - Setup a minimal working config (see bellow)
 - Build and run `go run haproxy-exporter.go`

## Usage
```
haproxy-exporter [flags]

Flags:
      --config string   config file to use
      --listen string   listen address (default "127.0.0.1:9100")
  -v, --verbose         verbose output
```

## Configuration
HAProxy-Exporter come with a simple default [config file](config.yaml).

Configuration is load and override in the following order:
 - /etc/haproxy-exporter/config.yaml
 - ~/haproxy-exporter/config.yaml
 - ./config.yaml
 - config filepath from command line

### Definitions
Config is composed of three main parts and some config fields:

#### Sources
HAProxy-Exporter can have one to many HAProxy stats sources. A *source* is defined as follow:
``` yaml
sources: # Sources definitions
  - uri: http://localhost/haproxy?stats;csv # HTTP stats uri
    labels: # Labels are added to every metrics (Optional)
      label_name : label_value # Label definition
  - uri: http://user:pass@haproxy.example.com/haproxy?stats;csv # HTTP with basic auth stats uri
  - uri: unix:/run/haproxy/admin.sock # Socket stats uri
```

#### Metrics
HAProxy-Exporter can expose some or all HAProxy stats:
``` yaml
metrics: # Metrics to collect (Optional, all if unset)
  - qcur
  - qmax
  - scur
  - smax
  - slim
  - stot
  - bin
  - bout
  - dreq
  - dresp
  - ereq
  - econ
  - eresp
  - wretr
  - wredis
  - chkfail
  - chkdown
  - downtime
  - qlimit
  - rate
  - rate_lim
  - rate_max
  - hrsp_1xx
  - hrsp_2xx
  - hrsp_3xx
  - hrsp_4xx
  - hrsp_5xx
  - hrsp_other
  - req_rate
  - req_rate_max
  - req_tot
  - cli_abrt
  - srv_abrt
```

#### Labels
HAProxy-Exporter can add static labels to collected metrics. A *label* is defined as follow:
``` yaml
labels: # Labels definitions (Optional)
  label_name: label_value # Label definition             (Required)
```

#### Parameters
HAProxy-Exporter can be customized through parameters. See available parameters bellow:
``` yaml
# parameters definitions (Optional)
scanDuration: 1000 # Duration within all the sources should be scraped (Optional, default: 1000)
maxConcurrent: 200 # Max concurrent scrape allowed (Optional, default: 50)
scrapeTimeout: 5000 # Stats fetch timeout (Optional, default: 5000)
flushPath: /opt/beamium/sinks/warp- # Path to flush metrics + filename header (Optional, default: no flush)
flushPeriod: 10000 # Flush period (Optional, 10000)
```

## Contributing
Instructions on how to contribute to HAProxy-Exporter are available on the [Contributing][Contributing] page.

## Get in touch

- Twitter: [@notd33d33](https://twitter.com/notd33d33)

[Contributing]: CONTRIBUTING.md
