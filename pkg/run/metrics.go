/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package run

import (
	"expvar"
	"strings"
	"sync"

	armonmetrics "github.com/armon/go-metrics"
	hashimetrics "github.com/hashicorp/go-metrics"
	hashiprom "github.com/hashicorp/go-metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

/*
MetricsBridge surfaces the raft and badger runtime metrics on the existing
prometheus /metrics endpoint (servion's MetricsHandler serves the default
registry):

  - hashicorp/raft reports through the go-metrics globals (leader last-contact,
    commit/apply latency, fsm pending, snapshot timings, …). A PrometheusSink is
    installed for BOTH go-metrics generations (armon and hashicorp) because raft
    emits via its compat layer, whose target depends on build tags — installing
    both covers either routing.
  - badger publishes expvar counters (badger_*) at package init; a collector
    snapshots them into prometheus metrics on scrape (expvar maps become a
    "key"-labeled series).

Installation is process-wide and once-only: servion's run command can rebuild the
bean graph on restart, so PostConstruct tolerates re-runs.
*/
type MetricsBridge struct {
	Log *zap.Logger `inject:""`
}

var metricsBridgeOnce sync.Once

func (t *MetricsBridge) BeanName() string { return "metrics-bridge" }

func (t *MetricsBridge) PostConstruct() (err error) {
	metricsBridgeOnce.Do(func() { err = t.install() })
	return err
}

func (t *MetricsBridge) install() error {
	// One prometheus sink; both go-metrics globals feed it (raft's compat layer
	// routes to the armon global by default, or the hashicorp one when built with
	// the hashicorpmetrics tag — either way the metrics land in this collector).
	hsink, err := hashiprom.NewPrometheusSink()
	if err != nil {
		return xerrors.Errorf("prometheus sink: %w", err)
	}
	hcfg := hashimetrics.DefaultConfig("consensusdb")
	hcfg.EnableHostname = false
	if _, err := hashimetrics.NewGlobal(hcfg, hsink); err != nil {
		return xerrors.Errorf("hashicorp go-metrics global: %w", err)
	}
	acfg := armonmetrics.DefaultConfig("consensusdb")
	acfg.EnableHostname = false
	if _, err := armonmetrics.NewGlobal(acfg, &armonSinkAdapter{sink: hsink}); err != nil {
		return xerrors.Errorf("armon go-metrics global: %w", err)
	}

	// badger expvar → prometheus.
	if err := prometheus.Register(&expvarCollector{prefix: "badger_"}); err != nil {
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			return xerrors.Errorf("register badger expvar collector: %w", err)
		}
	}
	t.Log.Info("MetricsBridgeInstalled",
		zap.String("raft", "go-metrics prometheus sinks (armon+hashicorp)"),
		zap.String("badger", "expvar collector (badger_*)"))
	return nil
}

// armonSinkAdapter lets the legacy armon/go-metrics global (raft's default
// compat routing) emit into the single hashicorp prometheus sink, so only one
// collector is ever registered on the default registry.
type armonSinkAdapter struct {
	sink hashimetrics.MetricSink
}

func hashiLabels(labels []armonmetrics.Label) []hashimetrics.Label {
	out := make([]hashimetrics.Label, len(labels))
	for i, l := range labels {
		out[i] = hashimetrics.Label{Name: l.Name, Value: l.Value}
	}
	return out
}

func (t *armonSinkAdapter) SetGauge(key []string, val float32) { t.sink.SetGauge(key, val) }
func (t *armonSinkAdapter) SetGaugeWithLabels(key []string, val float32, labels []armonmetrics.Label) {
	t.sink.SetGaugeWithLabels(key, val, hashiLabels(labels))
}
func (t *armonSinkAdapter) EmitKey(key []string, val float32)     { t.sink.EmitKey(key, val) }
func (t *armonSinkAdapter) IncrCounter(key []string, val float32) { t.sink.IncrCounter(key, val) }
func (t *armonSinkAdapter) IncrCounterWithLabels(key []string, val float32, labels []armonmetrics.Label) {
	t.sink.IncrCounterWithLabels(key, val, hashiLabels(labels))
}
func (t *armonSinkAdapter) AddSample(key []string, val float32) { t.sink.AddSample(key, val) }
func (t *armonSinkAdapter) AddSampleWithLabels(key []string, val float32, labels []armonmetrics.Label) {
	t.sink.AddSampleWithLabels(key, val, hashiLabels(labels))
}

// expvarCollector exports every expvar under prefix as an untyped prometheus
// metric, snapshotted at scrape time. Int/Float vars map 1:1; Map vars fan out
// into one series per entry with a "key" label. It reports nothing from
// Describe, making it an unchecked collector — names stay stable because badger
// registers its expvars once at package init.
type expvarCollector struct {
	prefix string
}

func (c *expvarCollector) Describe(chan<- *prometheus.Desc) {}

func (c *expvarCollector) Collect(ch chan<- prometheus.Metric) {
	expvar.Do(func(kv expvar.KeyValue) {
		if !strings.HasPrefix(kv.Key, c.prefix) {
			return
		}
		name := sanitizeMetricName(kv.Key)
		switch v := kv.Value.(type) {
		case *expvar.Int:
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(name, "badger expvar "+kv.Key, nil, nil),
				prometheus.UntypedValue, float64(v.Value()))
		case *expvar.Float:
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(name, "badger expvar "+kv.Key, nil, nil),
				prometheus.UntypedValue, v.Value())
		case *expvar.Map:
			desc := prometheus.NewDesc(name, "badger expvar "+kv.Key, []string{"key"}, nil)
			v.Do(func(entry expvar.KeyValue) {
				switch ev := entry.Value.(type) {
				case *expvar.Int:
					ch <- prometheus.MustNewConstMetric(desc, prometheus.UntypedValue, float64(ev.Value()), entry.Key)
				case *expvar.Float:
					ch <- prometheus.MustNewConstMetric(desc, prometheus.UntypedValue, ev.Value(), entry.Key)
				}
			})
		}
	})
}

// sanitizeMetricName maps an expvar key onto the prometheus name alphabet.
func sanitizeMetricName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == ':':
			return r
		default:
			return '_'
		}
	}, name)
}
