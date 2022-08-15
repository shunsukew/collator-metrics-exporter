package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/machinebox/graphql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	port = 9102
)

type Endpoint struct {
	Indexer   string
	Substrate string
}

var (
	networkEndpoints = map[string]Endpoint{
		"Astar": Endpoint{
			Indexer:   "https://api.subquery.network/sq/bobo-k2/collator-indexer-v2",
			Substrate: "https://astar.public.blastapi.io",
		},
		"Shiden": Endpoint{
			Indexer:   "https://api.subquery.network/sq/bobo-k2/shiden-colator-indexer-v2",
			Substrate: "https://shiden.public.blastapi.io",
		},
		"Shibuya": Endpoint{
			Indexer:   "https://api.subquery.network/sq/bobo-k2/shibuya-collator-indexer",
			Substrate: "https://shibuya.public.blastapi.io",
		},
	}
)

// Gueage
var (
	blockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "block_production_count",
			Help: "Block Productuion Count in the days before", // not last 24 hours because of subquery specification limitation.
		},
		[]string{"network", "date", "name", "address"},
	)

	missedBlockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "missed_block_production_count",
			Help: "Missed Block Productuion Count in the days before",
		},
		[]string{"network", "date", "name", "address"},
	)

	blockExtrinsicsGuage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "block_extrinsics_count",
			Help: "Block Extrinsics Count",
		},
		[]string{"network", "block_number"},
	)

	blockWeightRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "block_weight_ratio",
			Help: "Block Weight Ratio",
		},
		[]string{"network", "block_number"},
	)
)

type BlockProductionsResponseData struct {
	BlockProductions BlockProductions `json:"blockProductions"`
}

type BlockProductions struct {
	Nodes []*BlockProduction `json:"nodes"`
}

type BlockProduction struct {
	CollatorID     string   `json:"collatorId"`
	Collator       Collator `json:"collator"`
	BlocksProduced uint32   `json:"blocksProduced"`
	BlocksMissed   uint32   `json:"blocksMissed"`
}

type Collator struct {
	Name string `json:"name"`
}

type BlockFillingsResponseData struct {
	BlockFillings BlockFillings `json:"blocks"`
}

type BlockFillings struct {
	Nodes []*BlockFilling `json:"nodes"`
}

type BlockFilling struct {
	BlockNumber     string  `json:"id"`
	ExtrinsicsCount uint32  `json:"extrinsicsCount"`
	WeightRatio     float64 `json:"weightRatio"`
}

func updateBlockProductionGuage() {
	var lastUnixDay int64
	for {
		now := time.Now()
		// if already updated metrics today
		unixDay := now.Unix() / 86400
		if lastUnixDay == unixDay {
			time.Sleep(3600 * time.Second)
			continue
		}

		yesterday := now.AddDate(0, 0, -1).Format("20060102")
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

		for network, endpoint := range networkEndpoints {
			graphQLClient := graphql.NewClient(endpoint.Indexer)

			log.Println(fmt.Printf("Requesting %s ...", endpoint.Indexer))
			var respData BlockProductionsResponseData
			if err := graphQLClient.Run(context.Background(), req, &respData); err != nil {
				log.Fatal(err)
			}

			blockProductionGauge.DeletePartialMatch(prometheus.Labels{"network": network})
			missedBlockProductionGauge.DeletePartialMatch(prometheus.Labels{"network": network})
			for _, node := range respData.BlockProductions.Nodes {
				blockProductionGauge.With(prometheus.Labels{"network": network, "date": yesterday, "name": node.Collator.Name, "address": node.CollatorID}).Set(float64(node.BlocksProduced))
				missedBlockProductionGauge.With(prometheus.Labels{"network": network, "date": yesterday, "name": node.Collator.Name, "address": node.CollatorID}).Set(float64(node.BlocksMissed))
			}
		}

		lastUnixDay = unixDay
	}
}

func updateBlockFillingsGuage() {
	for {
		for network, endpoint := range networkEndpoints {
			api, err := gsrpc.NewSubstrateAPI(endpoint.Substrate)
			if err != nil {
				log.Fatal(err)
			}

			latestBlockNum, err := api.RPC.Chain.GetBlockLatest()
			if err != nil {
				log.Fatal(err)
			}

			// hard coded. Approx - 3600 blocks * 12 sec = 43200 (= 0.5day)
			blockNumSince := uint32(latestBlockNum.Block.Header.Number) - 3600

			query := fmt.Sprintf(`query {
				blocks (filter: {
				  id: {
					greaterThan: "%d",
				  }
				}, orderBy: ID_ASC) {
					nodes {
						id
						extrinsicsCount
						weightRatio
					}
				}
			}`, blockNumSince)

			req := graphql.NewRequest(query)
			req.Header.Set("Content-Type", "application/json")

			endpoint, ok := networkEndpoints[network]
			if !ok {
				log.Fatalf("unknown network %s", network)
			}
			graphQLClient := graphql.NewClient(endpoint.Indexer)

			log.Println(fmt.Printf("Requesting %s ...", endpoint.Indexer))
			var respData BlockFillingsResponseData
			if err := graphQLClient.Run(context.Background(), req, &respData); err != nil {
				log.Fatal(err)
			}

			blockExtrinsicsGuage.DeletePartialMatch(prometheus.Labels{"network": network})
			blockWeightRatio.DeletePartialMatch(prometheus.Labels{"network": network})
			for _, node := range respData.BlockFillings.Nodes {
				blockExtrinsicsGuage.With(prometheus.Labels{"network": network, "block_number": node.BlockNumber}).Set(float64(node.ExtrinsicsCount))
				blockWeightRatio.With(prometheus.Labels{"network": network, "block_number": node.BlockNumber}).Set(node.WeightRatio)
			}
		}

		time.Sleep(1 * time.Hour)
	}
}

func main() {
	go updateBlockProductionGuage()
	go updateBlockFillingsGuage()

	log.Println("Starting prometheus metric server")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
