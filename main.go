package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
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
			Substrate: "wss://rpc.astar.network",
		},
		"Shiden": Endpoint{
			Indexer:   "https://api.subquery.network/sq/bobo-k2/shiden-collator-indexer",
			Substrate: "wss://rpc.shiden.astar.network",
		},
		"Shibuya": Endpoint{
			Indexer:   "https://api.subquery.network/sq/bobo-k2/shibuya-collator-indexer",
			Substrate: "wss://rpc.shibuya.astar.network",
		},
	}
	networkGraphQLClients = map[string]*graphql.Client{} // read only by goroutines
	networkLastBlockNums  = map[string]uint32{}          // no concurrent update expected
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

		for network, client := range networkGraphQLClients {
			var respData BlockProductionsResponseData
			if err := client.Run(context.Background(), req, &respData); err != nil {
				log.Fatal(err)
			}

			blockProductionGauge.Delete(prometheus.Labels{"network": network})
			missedBlockProductionGauge.Delete(prometheus.Labels{"network": network})
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
		for network, lastBlockNum := range networkLastBlockNums {
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
			}`, lastBlockNum)

			req := graphql.NewRequest(query)
			req.Header.Set("Content-Type", "application/json")

			graphQLClient, ok := networkGraphQLClients[network]
			if !ok {
				log.Fatalf("unknown network %s", network)
			}

			var respData BlockFillingsResponseData
			if err := graphQLClient.Run(context.Background(), req, &respData); err != nil {
				log.Fatal(err)
			}

			for _, node := range respData.BlockFillings.Nodes {
				blockExtrinsicsGuage.With(prometheus.Labels{"network": network, "block_number": node.BlockNumber}).Set(float64(node.ExtrinsicsCount))
				blockWeightRatio.With(prometheus.Labels{"network": network, "block_number": node.BlockNumber}).Set(node.WeightRatio)

				blockNumber, err := strconv.ParseUint(node.BlockNumber, 10, 32)
				if err != nil {
					log.Fatal(err)
				}

				if networkLastBlockNums[network] < uint32(blockNumber) {
					networkLastBlockNums[network] = uint32(blockNumber)
				}
			}
		}

		time.Sleep(15 * time.Second)
	}
}

func init() {
	for network, endpoint := range networkEndpoints {
		// GraphQL
		graphQLClient := graphql.NewClient(endpoint.Indexer)
		networkGraphQLClients[network] = graphQLClient

		// Substrate API
		api, err := gsrpc.NewSubstrateAPI(endpoint.Substrate)
		if err != nil {
			log.Fatal(err)
		}

		latestBlockNum, err := api.RPC.Chain.GetBlockLatest()
		if err != nil {
			log.Fatal(err)
		}

		// hard coded. Approx - 7200 blocks * 12 sec = 86400 (= 1day)
		lastBlockNum := uint32(latestBlockNum.Block.Header.Number) - 7200
		networkLastBlockNums[network] = lastBlockNum
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
