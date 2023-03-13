#!/bin/bash -eux

cd /usr/local/bin

while ! [ -s collator-metrics-exporter ]; do
  rm -rf collator-metrics-exporter
  wget https://github.com/shunsukew/collator-metrics-exporter/releases/download/v0.7.1/collator-metrics-exporter -O collator-metrics-exporter
  sleep 5
done

chmod u+x collator-metrics-exporter

export PORT=9102
export DATA_SOURCE="host=127.0.0.1 port=5432 user=postgres password=passw0rd dbname=blocks sslmode=disable"
export ASTAR_INDEXER_ENDPOINT=https://api.subquery.network/sq/bobo-k2/astar-collator-indexer
export ASTAR_NODE_ENDPOINT=https://evm.astar.network
export SHIDEN_INDEXER_ENDPOINT=https://api.subquery.network/sq/bobo-k2/shiden-collator-indexer
export SHIDEN_NODE_ENDPOINT=https://evm.shiden.astar.network
export SHIBUYA_INDEXER_ENDPOINT=https://api.subquery.network/sq/bobo-k2/shibuya-collator-indexer
export SHIBUYA_NODE_ENDPOINT=https://evm.shibuya.astar.network

./collator-metrics-exporter
