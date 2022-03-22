# collator-metrics-exporter

*This is temporary solution

Astar collator metrics exporter.
This queries data from SubQuery and export them as Prometheus formatted way.

## Metrics Explanations

- `block_production_count_1h` Count of blocks that was produced by each collators last hour
  ```
  block_production_count_1h{} 1
  ```

- `block_production_count_1d` Count of blocks that was produced by each collators last hour
  ```
  block_production_count_1d{} 1
  ```

- `missed_block_production_count_1h` Counts of blocks each collator missed to produce last hour
  ```
  missed_block_production_count_1h{} 1
  ```

- `missed_block_production_count_1d` Counts of blocks each collator missed to produce last hour
  ```
  missed_block_production_count_1d{} 1

- `produced_blocks` Block Information
  ```
  produced_blocks{collator_name: xxx, collator_address: yyy, block_height: 10000} 0
  ```
