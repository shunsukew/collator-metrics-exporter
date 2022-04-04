# collator-metrics-exporter

*This is for temporary usage.
Once Substrate implements sufficent prometheus metrics which allow us to achieve the same goal, This will be no more required.
We now only have SubQuery https://explorer.subquery.network/subquery/bobo-k2/collator-indexer?stage=true

We want
- Grafana block production board for each collator
- Grafana missed block production board for each collator

Astar collator metrics exporter.
This just queries data from SubQuery and export them as Prometheus formatted way.

## Metrics Explanations

- `block_production_count` Count of blocks that was produced by each collators last 24 hours
  ```
  block_production_count{} 1
  ```

- `missed_block_production_count` Counts of blocks each collator missed to produce last 24 hours
  ```
  missed_block_production_count{} 1

- `produced_blocks` Block Information
  ```
  produced_blocks{collator_name: xxx, collator_address: yyy, block_height: 10000} 0
  ```

## How to run
1. put collator-metrics-exporter service at /etc/systemd/system/
2. put start-collator-metrics-exporter.sh at /opt/bin/
3. sudo systemctl enable collator-metrics-exporter
