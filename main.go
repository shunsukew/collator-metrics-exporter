package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/machinebox/graphql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Endpoint struct {
	Indexer   string
	Substrate string
}

var (
	port             uint32
	networkEndpoints = map[string]*Endpoint{}
)

// Gueage
var (
	blockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "block_production_count",
			Help: "Block Productuion Count in the days before", // not last 24 hours because of subquery specification limitation.
		},
		[]string{"network", "address"},
	)

	missedBlockProductionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "missed_block_production_count",
			Help: "Missed Block Productuion Count in the days before",
		},
		[]string{"network", "address"},
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

type BlockProductionResponseData struct {
	Data BlockRealTimeData `json:"blockRealTimeData"`
}

type BlockRealTimeData struct {
	BlockProductions []*BlockProduction `json:"groupedAggregates"`
}

type BlockProductions struct {
	Nodes []*BlockProduction `json:"nodes"`
}

type BlockProduction struct {
	Keys               []string            `json:"keys"`
	BlockDistinctCount *BlockDistinctCount `json:"distinctCount"`
}

type BlockDistinctCount struct {
	BlockNumber string `json:"blockNumber"`
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
	// Update metrics hourly.
	var lastUnixHour int64
	for {
		now := time.Now()
		// if already updated metrics this hour, sleep
		currentHour := now.Truncate(time.Hour)
		unixHour := currentHour.Unix()
		if unixHour == lastUnixHour {
			time.Sleep(10 * time.Minute)
			continue
		}

		since := currentHour.Add(-24 * time.Hour)
		until := currentHour

		query := fmt.Sprintf(`query {
			blockRealTimeData(filter: {
				and: [
				  {timestamp: {greaterThanOrEqualTo: "%d"}},
				  {timestamp: {lessThan: "%d"}},
				]
			}) {
			  groupedAggregates(groupBy: [COLLATOR_ADDRESS, STATUS]) {
				keys
				distinctCount {
				  blockNumber
				}
				average{
				  weightRatio
				}
			  }
			}
		}`, since.UnixMilli(), until.UnixMilli())

		req := graphql.NewRequest(query)
		req.Header.Set("Content-Type", "application/json")

		for network, endpoint := range networkEndpoints {
			graphQLClient := graphql.NewClient(endpoint.Indexer)

			log.Println(fmt.Printf("Requesting %s ...", endpoint.Indexer))
			var respData *BlockProductionResponseData
			if err := graphQLClient.Run(context.Background(), req, &respData); err != nil {
				log.Fatal(err)
			}

			for _, blockProduction := range respData.Data.BlockProductions {
				if len(blockProduction.Keys) != 2 {
					continue
				}

				blocksCount, err := strconv.Atoi(blockProduction.BlockDistinctCount.BlockNumber)
				if err != nil {
					log.Fatal(err)
				}
				if blockProduction.Keys[1] == "Produced" {
					blockProductionGauge.With(prometheus.Labels{"network": network, "address": blockProduction.Keys[0]}).Set(float64(blocksCount))
				} else {
					missedBlockProductionGauge.With(prometheus.Labels{"network": network, "address": blockProduction.Keys[0]}).Set(float64(blocksCount))
				}
			}
		}

		lastUnixHour = unixHour
	}
}

// func updateBlockFillingGuage() {
// for {
// for network, endpoint := range networkEndpoints {
// api, err := gsrpc.NewSubstrateAPI(endpoint.Substrate)
// if err != nil {
// log.Fatal(err)
// }

// latestBlockNum, err := api.RPC.Chain.GetBlockLatest()
// if err != nil {
// log.Fatal(err)
// }

// // hard coded. Approx - 3600 blocks * 12 sec = 43200 (= 0.5day)
// blockNumSince := uint32(latestBlockNum.Block.Header.Number) - 3600

// query := fmt.Sprintf(`query {
// blocks (filter: {
// id: {
// greaterThan: "%d",
// }
// }, orderBy: ID_ASC) {
// nodes {
// id
// extrinsicsCount
// weightRatio
// }
// }
// }`, blockNumSince)

// req := graphql.NewRequest(query)
// req.Header.Set("Content-Type", "application/json")

// endpoint, ok := networkEndpoints[network]
// if !ok {
// log.Fatalf("unknown network %s", network)
// }
// graphQLClient := graphql.NewClient(endpoint.Indexer)

// log.Println(fmt.Printf("Requesting %s ...", endpoint.Indexer))
// var respData BlockFillingsResponseData
// if err := graphQLClient.Run(context.Background(), req, &respData); err != nil {
// log.Fatal(err)
// }

// blockExtrinsicsGuage.DeletePartialMatch(prometheus.Labels{"network": network})
// blockWeightRatio.DeletePartialMatch(prometheus.Labels{"network": network})
// for _, node := range respData.BlockFillings.Nodes {
// blockExtrinsicsGuage.With(prometheus.Labels{"network": network, "block_number": node.BlockNumber}).Set(float64(node.ExtrinsicsCount))
// blockWeightRatio.With(prometheus.Labels{"network": network, "block_number": node.BlockNumber}).Set(node.WeightRatio)
// }
// }

// time.Sleep(1 * time.Hour)
// }
// }

func init() {
	portTmp, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		log.Fatal(err)
	}
	port = uint32(portTmp)

	astarIndexerEndpoint := os.Getenv("ASTAR_INDEXER_ENDPOINT")
	if astarIndexerEndpoint == "" {
		log.Fatal("Astar indexer endpoint is not set")
	}
	astarNodeEndpoint := os.Getenv("ASTAR_NODE_ENDPOINT")
	if astarNodeEndpoint == "" {
		log.Fatal("Astar node endpoint is not set")
	}
	// shidenIndexerEndpoint := os.Getenv("SHIDEN_INDEXER_ENDPOINT")
	// if shidenIndexerEndpoint == "" {
	// log.Fatal("Shiden indexer endpoint is not set")
	// }
	// shidenNodeEndpoint := os.Getenv("SHIDEN_NODE_ENDPOINT")
	// if shidenNodeEndpoint == "" {
	// log.Fatal("Shiden node endpoint is not set")
	// }
	// shibuyaIndexerEndpoint := os.Getenv("SHIBUYA_INDEXER_ENDPOINT")
	// if shibuyaIndexerEndpoint == "" {
	// log.Fatal("Shibuya indexer endpoint is not set")
	// }
	// shibuyaNodeEndpoint := os.Getenv("SHIBUYA_NODE_ENDPOINT")
	// if shibuyaNodeEndpoint == "" {
	// log.Fatal("Shibuya node endpoint is not set")
	// }

	networkEndpoints["Astar"] = &Endpoint{Indexer: astarIndexerEndpoint, Substrate: astarNodeEndpoint}
	// networkEndpoints["Shiden"] = &Endpoint{Indexer: shidenIndexerEndpoint, Substrate: shidenNodeEndpoint}
	// networkEndpoints["Shibuya"] = &Endpoint{Indexer: shibuyaIndexerEndpoint, Substrate: shibuyaNodeEndpoint}
}

func main() {
	go updateBlockProductionGuage()
	// go updateBlockFillingGuage()

	log.Println("Starting prometheus metric server")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/metrics", promhttp.Handler())
	log.Println(fmt.Sprintf("Listening on port %d", port))
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
