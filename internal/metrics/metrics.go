package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	IngestionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tuma_ingestion_total",
		Help: "Events ingested by connection and outcome",
	}, []string{"connection_id", "outcome"})

	DeliveryLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tuma_delivery_latency_ms",
		Help:    "Outbound delivery latency in milliseconds",
		Buckets: prometheus.ExponentialBuckets(10, 2, 10),
	}, []string{"connection_id", "status"})

	DLQDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tuma_dlq_depth",
		Help: "Number of open issues",
	})

	ConnectionErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tuma_connection_errors_total",
		Help: "Delivery errors per connection",
	}, []string{"connection_id"})
)

func SetDLQDepth(n float64) {
	DLQDepth.Set(n)
}
