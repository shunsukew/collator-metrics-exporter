# collator-metrics-exporter

*This is temporary solution.
once Substrate implements sufficent prometheus metrics which allow us to achieve the same goal, This will be no more required.

We want
- Grafana block production board for each collator
- Grafana missed block production board for each collator

Astar collator metrics exporter.
This queries data from SubQuery and export them as Prometheus formatted way.

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
