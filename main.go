package main

import (
	"context"
	"fmt"
	"log"
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

// Gueage
var (
	blockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "block_production_count",
			Help: "Block Productuion Count For Each Collator Last 24 Hours",
		},
		[]string{"date", "name", "address"},
	)

	missedBlockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "missed_block_production_count",
			Help: "Missed Block Productuion Count For Each Collator Last 24 Hours",
		},
		[]string{"date", "name", "address"},
	)
)

//type ResponseData struct {
//Data Data `json:"data"`
//}

type ResponseData struct {
	BlockProductions BlockProductions `json:"blockProductions"`
}

type BlockProductions struct {
	Nodes []*Node `json:"nodes"`
}

type Node struct {
	CollatorID     string   `json:"collatorId"`
	Collator       Collator `json:"collator"`
	BlocksProduced uint32   `json:"blocksProduced"`
	BlocksMissed   uint32   `json:"blocksMissed"`
}

type Collator struct {
	Name string `json:"name"`
}

func updateBlockProductionGuage() {
	var lastUnixDay int64
	for {
		now := time.Now()
		yesterday := now.AddDate(0, 0, -1).Format("20060102")
		graphQLClient := graphql.NewClient("https://api.subquery.network/sq/bobo-k2/collator-indexer__Ym9ib")
		query := fmt.Sprintf(`query {
			blockProductions(filter: {
			  dayId: {
			    equalTo: "%s"
			  }
			}) {
			  nodes {
			    collatorId,
			    collator{
			      name
			    },
			    blocksProduced,
			    blocksMissed
			  }
			}
		      }`, yesterday)
		req := graphql.NewRequest(query)
		req.Header.Set("Content-Type", "application/json")

		// if already updated metrics today
		unixDay := now.Unix() / 86400
		if lastUnixDay == unixDay {
			time.Sleep(3600 * time.Second)
			continue
		}

		var respData ResponseData
		if err := graphQLClient.Run(context.Background(), req, &respData); err != nil {
			log.Fatal(err)
		}

		for _, node := range respData.BlockProductions.Nodes {
			blockProductionGauge.With(prometheus.Labels{"date": yesterday, "name": node.Collator.Name, "address": node.CollatorID}).Set(float64(node.BlocksProduced))
			missedBlockProductionGauge.With(prometheus.Labels{"date": yesterday, "name": node.Collator.Name, "address": node.CollatorID}).Set(float64(node.BlocksMissed))
		}

		lastUnixDay = unixDay
	}
}

func main() {
	// block
	go updateBlockProductionGuage()

	log.Println("Starting prometheus metric server")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
