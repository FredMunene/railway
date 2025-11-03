package server

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metricsRegistry struct {
	registry           *prometheus.Registry
	mintIntentsTotal   *prometheus.CounterVec
	callbacksTotal     *prometheus.CounterVec
	retryAttemptsTotal *prometheus.CounterVec
	dlqDepth           prometheus.Gauge
}

func newMetricsRegistry() *metricsRegistry {
	mint := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatrails_mint_intents_total",
		Help: "Total number of mint intent submissions",
	}, []string{"status"})

	callbacks := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatrails_callbacks_total",
		Help: "Total number of M-PESA callbacks processed",
	}, []string{"status"})

	retries := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fiatrails_retry_attempts_total",
		Help: "Retry attempts for callback execution",
	}, []string{"result"})

	dlq := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "fiatrails_dlq_depth",
		Help: "Number of items in the DLQ",
	})

	r := prometheus.NewRegistry()
	r.MustRegister(mint, callbacks, retries, dlq)

	return &metricsRegistry{
		registry:           r,
		mintIntentsTotal:   mint,
		callbacksTotal:     callbacks,
		retryAttemptsTotal: retries,
		dlqDepth:           dlq,
	}
}

func (m *metricsRegistry) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *metricsRegistry) incMint(status string) {
	m.mintIntentsTotal.WithLabelValues(status).Inc()
}

func (m *metricsRegistry) incCallback(status string) {
	m.callbacksTotal.WithLabelValues(status).Inc()
}

func (m *metricsRegistry) incRetry(result string) {
	m.retryAttemptsTotal.WithLabelValues(result).Inc()
}

func (m *metricsRegistry) setDLQDepth(depth int) {
	m.dlqDepth.Set(float64(depth))
}
