package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/machinebox/graphql"
)

const (
	port = 9102
)

type ResponseData struct {
}

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
			Name: "block_production_count",
			Help: "Block Productuion Count For Each Collator Last 24 Hours",
		},
		[]string{"sample"},
	)

	missedBlockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "missed_block_production_count",
			Help: "Missed Block Productuion Count For Each Collator Last 24 Hours",
		},
		[]string{"sample"},
	)
)

func updateRandomValue() {
	for {
		rand.Seed(time.Now().UnixNano())
		n := -1 + rand.Float64()*2
		exampleGauge.With(prometheus.Labels{"label_key": "label_val"}).Set(n)
		time.Sleep(10 * time.Second)
	}
}

func updateBlockProductionGuage(client *graphql.Client) {
	req := graphql.NewRequest(`
		query {
		  blockProductions(filter: {
		    dayId: {
		      equalTo: "${formattedDate}"
		    }
		  },
		  orderBy: BLOCKS_MISSED_DESC) {
		    nodes {
		      collatorId,
		      collator{
		        name
		      },
		      blocksProduced,
		      blocksMissed
		    }
		  }
	        }
		`)

	req.Var("key", "value")
	req.Header.Set("Cache-Control", "no-cache")

	for {
		ctx := context.Background()
		ctx, _ = context.WithTimeout(ctx, 60*time.Second)

		var respData ResponseData
		if err := client.Run(ctx, req, &respData); err != nil {
			log.Fatal(err)
		}

		time.Sleep(600 * time.Second)
	}
}

func main() {
	// configure subquery client
	graphQLClient := graphql.NewClient("https://api.subquery.network/sq/bobo-k2/collator-indexer__Ym9ib")

	// block

	go setRandomValue()

	log.Println("Starting prometheus metric server")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
