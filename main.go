package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	_ "github.com/lib/pq"
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

	dataSource string
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
)

type BlockProductionResponseData struct {
	Data BlockProductionsData `json:"blockRealTimeData"`
}

type BlockProductionsData struct {
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

type BlocksResponseData struct {
	Data BlockDatum `json:"blockRealTimeData"`
}

type BlockDatum struct {
	BlockDatum []*BlockData `json:"nodes"`
}

type BlockData struct {
	BlockNumber     string  `json:"blockNumber"`
	Timestamp       string  `json:"timestamp"`
	CollatorAddress string  `json:"collatorAddress"`
	ExtrinsicsCount uint32  `json:"extrinsicsCount"`
	Weight          string  `json:"weight"`
	WeightRatio     float32 `json:"weightRatio"`
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

func updateBlockFillingGuage() {
	db, err := sql.Open("postgres", dataSource)
	if err != nil {
		fmt.Println(err)
	}
	defer db.Close()

	for {
		for network, endpoint := range networkEndpoints {
			var lastBlockTimestamp int64
			time.Now().Unix()
			err = db.QueryRow("SELECT block_timestamp FROM blocks WHERE network = $1 ORDER BY block_number DESC LIMIT 1;", network).Scan(&lastBlockTimestamp)
			if err != nil && err != sql.ErrNoRows {
				log.Fatal(err)
			}

			if lastBlockTimestamp == 0 {
				api, err := gsrpc.NewSubstrateAPI(endpoint.Substrate)
				if err != nil {
					log.Fatal(err)
				}

				latestBlockNum, err := api.RPC.Chain.GetBlockLatest()
				if err != nil {
					log.Fatal(err)
				}

				// hard coded. Approx - 3600 blocks * 12 sec = 43200 (= 0.5day)
				lastBlockTimestamp = int64(latestBlockNum.Block.Header.Number) - 3600
			}

			query := fmt.Sprintf(`query {
				blockRealTimeData(filter: {
					and: [
						{timestamp: {greaterThanOrEqualTo: "%d"}}
						{status: {equalTo: Produced}}
					]
				}) {
					nodes {
						blockNumber
						timestamp
						collatorAddress
						extrinsicsCount
						weight
						weightRatio
					}
				}
			}`, lastBlockTimestamp)

			req := graphql.NewRequest(query)
			req.Header.Set("Content-Type", "application/json")

			endpoint, ok := networkEndpoints[network]
			if !ok {
				log.Fatalf("unknown network %s", network)
			}
			graphQLClient := graphql.NewClient(endpoint.Indexer)

			log.Println(fmt.Printf("Requesting %s ...", endpoint.Indexer))
			var respData *BlocksResponseData
			if err := graphQLClient.Run(context.Background(), req, &respData); err != nil {
				log.Fatal(err)
			}

			var blockNumber uint64
			var blockTimestamp uint64
			var weight uint64
			for _, block := range respData.Data.BlockDatum {
				blockNumber, err = strconv.ParseUint(block.BlockNumber, 10, 64)
				if err != nil {
					fmt.Println("HERE 1")
					fmt.Println(block.BlockNumber)
					log.Fatal(err)
				}
				blockTimestamp, err = strconv.ParseUint(block.Timestamp, 10, 64)
				if err != nil {
					fmt.Println("HERE 2")
					fmt.Println(blockTimestamp)
					log.Fatal(err)
				}
				weight, err = strconv.ParseUint(block.Weight, 10, 64)
				if err != nil {
					fmt.Println("HERE 3")
					fmt.Println(network)
					fmt.Println(lastBlockTimestamp)
					fmt.Println(block.Weight)
					log.Fatal(err)
				}

				_, err = db.Exec(
					"INSERT INTO blocks(network, block_number, block_timestamp, collator_address, extrinsics_count, weight, weight_ratio) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (network, block_number) DO UPDATE SET extrinsics_count=$8, weight=$9, weight_ratio=$10;",
					network,
					blockNumber,
					blockTimestamp,
					block.CollatorAddress,
					block.ExtrinsicsCount,
					weight,
					block.WeightRatio,
					block.ExtrinsicsCount,
					weight,
					block.WeightRatio,
				)
				if err != nil {
					log.Fatal(err)
				}
			}
		}

		time.Sleep(1 * time.Hour)
	}
}

func init() {
	portTmp, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		log.Fatal(err)
	}
	port = uint32(portTmp)

	dataSource = os.Getenv("DATA_SOURCE")
	if dataSource == "" {
		log.Fatal("data source is not set")
	}

	astarIndexerEndpoint := os.Getenv("ASTAR_INDEXER_ENDPOINT")
	if astarIndexerEndpoint == "" {
		log.Fatal("Astar indexer endpoint is not set")
	}
	astarNodeEndpoint := os.Getenv("ASTAR_NODE_ENDPOINT")
	if astarNodeEndpoint == "" {
		log.Fatal("Astar node endpoint is not set")
	}
	shidenIndexerEndpoint := os.Getenv("SHIDEN_INDEXER_ENDPOINT")
	if shidenIndexerEndpoint == "" {
		log.Fatal("Shiden indexer endpoint is not set")
	}
	shidenNodeEndpoint := os.Getenv("SHIDEN_NODE_ENDPOINT")
	if shidenNodeEndpoint == "" {
		log.Fatal("Shiden node endpoint is not set")
	}
	shibuyaIndexerEndpoint := os.Getenv("SHIBUYA_INDEXER_ENDPOINT")
	if shibuyaIndexerEndpoint == "" {
		log.Fatal("Shibuya indexer endpoint is not set")
	}
	shibuyaNodeEndpoint := os.Getenv("SHIBUYA_NODE_ENDPOINT")
	if shibuyaNodeEndpoint == "" {
		log.Fatal("Shibuya node endpoint is not set")
	}

	networkEndpoints["Astar"] = &Endpoint{Indexer: astarIndexerEndpoint, Substrate: astarNodeEndpoint}
	networkEndpoints["Shiden"] = &Endpoint{Indexer: shidenIndexerEndpoint, Substrate: shidenNodeEndpoint}
	networkEndpoints["Shibuya"] = &Endpoint{Indexer: shibuyaIndexerEndpoint, Substrate: shibuyaNodeEndpoint}
}

func main() {
	go updateBlockProductionGuage()
	go updateBlockFillingGuage()

	log.Println("Starting prometheus metric server")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/metrics", promhttp.Handler())
	log.Println(fmt.Sprintf("Listening on port %d", port))
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
