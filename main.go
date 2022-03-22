package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	port = 9102
)

var (
	exampleGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "example_number",
			Help: "Example Gauge",
		},
		[]string{"fuga"},
	)

	blockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "block_production_count_1d",
			Help: "Block Productuion Count For Each Collator Last 1 Day",
		},
		[]string{"sample"},
	)
)

func setRandomValue() {
	for {
		rand.Seed(time.Now().UnixNano())
		n := -1 + rand.Float64()*2
		exampleGauge.With(prometheus.Labels{"fuga": "fugafuga"}).Set(n)
		time.Sleep(10 * time.Second)
	}
}

func main() {
	go setRandomValue()

	log.Panicln("Starting prometheus metric server")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
