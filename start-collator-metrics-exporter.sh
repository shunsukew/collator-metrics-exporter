#!/bin/bash -eux

cd /usr/local/bin

while ! [ -s collator-metrics-exporter ]; do
  rm -rf collator-metrics-exporter
  wget https://github.com/shunsukew/collator-metrics-exporter/releases/download/v0.1.1/collator-metrics-exporter -O collator-metrics-exporter
  sleep 5
done

chmod u+x collator-metrics-exporter

./collator-metrics-exporter
