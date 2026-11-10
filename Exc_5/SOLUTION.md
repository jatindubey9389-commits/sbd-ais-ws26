# Exercise 5: Jatin Dubey

## Stage A: Today's system

### 1. Detector + plant with 20 machines (90 s)

- Rows in the table for the run: 18,000
- Readings the detector read: 15,094
- `missing`: 2,906

The detector kept up (backlog 0), but `missing` kept growing.

Are the missing readings in the table? Yes. 18,000 - 15,094 = 2,906, exactly the number of missing readings. They are in the table; the detector just never read them.

Why did the detector never see them? A row gets its `id` at INSERT, but other connections only see it after COMMIT, and the gateways commit at different times. In the timeline, gateway 1 inserts ids 100 to 114 but has not committed yet, and gateway 2 inserts 115 to 129 and commits at once. When the detector polls in between, it sees only 115 to 129 and remembers `last_id = 129`. Then gateway 1 commits 100 to 114. The next poll asks for `id > 129`, so rows 100 to 114 are skipped forever. Ids do not become visible in id order, so "highest id so far" loses rows.

### 2. Detector + full plant (60 machines, 600 readings/s, 60 s)

| time | backlog | delay | missing |
|---|---:|---:|---:|
| 5 s | 840 rows | 1.5 s | 16 |
| 20 s | 6,605 rows | 11.1 s | 61 |
| 50 s | 16,580 rows | 29.1 s | 61 |

- `delay` and `backlog` grow the whole time: the delay goes up by about 3 s every 5 s.
- `missing` stops growing.

Why? The model needs 3 ms per reading, so one detector can do about 333 readings/s (I measured about 400/s). The plant sends 600/s, so the detector falls further behind every second. Because it is behind, every row it reads was committed long ago, so the commit gap from step 1 no longer happens and `missing` stays the same. But the alerts come far too late. The first fault started at 40 s, and when I stopped the plant the detector was about 29 s behind and had found 0 anomalies. After the plant stopped, it only worked through its backlog.

## Stage B: Decision record

Options

| | 1: PostgreSQL + several pollers | 2: gateways write to Kafka |
|---|---|---|
| R1 scale | At risk. 100,000 INSERTs/s plus N pollers querying the same table make PostgreSQL the bottleneck. | Met. More partitions and more detector instances in one group. |
| R2 complete, ordered | At risk. The commit-order gap from Stage A stays. | Met. Key = `machine_id` puts each machine in one partition, in order. Offsets mean nothing is skipped. |
| R3 fresh | At risk. Polling interval plus DB load. | Met. Measured p95 = 0.09 s. |
| R4 independent consumers | Fails. Each new consumer adds queries on PostgreSQL. | Met. Each consumer has its own group and offsets, and adds no PostgreSQL load. |

Decision: Option 2. The gateways write to topic `readings` (12 partitions, key = `machine_id`). The detector instances share group `detector` and publish to `anomalies` (key = `machine_id`). The archive and the dashboard each use their own consumer group. PostgreSQL stays the system of record for the machine registry.

Numbers

- 600 readings/s x 3 ms = 1.8 s of work per second. That means at least 2 instances; I run 3.
- 100,000 readings/s x 3 ms = 300 s of work per second. That means at least 300 instances, about 400 with headroom, and at least as many partitions as instances.
- Tipping point: about where one detector can no longer keep up (333 readings/s with this model), or as soon as a second consumer needs the raw data. One reading per minute is far below that.

Settings

- Producer: `acks=all`, `enable.idempotence=true` (no lost, duplicated or reordered readings), `linger.ms=10`, `lz4` compression, and `murmur2_random` partitioner (the same partitioning as the Kafka CLI tools).
- Detector: `auto.offset.reset=latest`, because a live detector should start with the current data, not replay old readings and old runs.
- When partitions are revoked, the detector commits its offsets and calls `model.forget()` for the machines in those partitions. This way, there is no double processing and no stale `next_seq` if a partition comes back.

## Stage C: Build the core

### Commands to create the topics

```bash
docker exec -it kafka kafka-topics.sh --bootstrap-server localhost:9092 \
  --create --topic readings --partitions 12 --replication-factor 1

docker exec -it kafka kafka-topics.sh --bootstrap-server localhost:9092 \
  --create --topic anomalies --partitions 3 --replication-factor 1
```

### Code

- `guardian/plant.py`: `KafkaSink`
- `guardian/detector_service.py`

Started with 3 instances (`det-1`, `det-2`, `det-3`), then `python3 plant.py --sink kafka` for about 3.5 minutes. Each instance got 4 partitions.

### Scoreboard output

```
Run 1794345968: started 22:26:08 (3.4 min ago), 60 machines x 10 Hz, 3 faults so far

Detection  (topic anomalies: 1,155 anomaly events, models ['v1'])
   #  machine   start  detected   delay
   1  M-029       40s       yes   7.9 s
   2  M-034      100s       yes   7.5 s
   3  M-015      160s       yes   6.7 s

  faults detected: 3/3, mean delay 7.4 s; false alarms (episodes on healthy machines): 0
  latency reading -> anomaly event: p50 0.05 s, p95 0.09 s, max 0.09 s
  duplicate anomaly events (same anomaly_id): 0

Detector instances  (model telemetry, this run)
  det-1          pid 2755    v1  readings    48,783  missing       0  out-of-order      0  machines   25  alive
  det-2          pid 2757    v1  readings    37,082  missing       0  out-of-order      0  machines   19  alive
  det-3          pid 2759    v1  readings    31,232  missing       0  out-of-order      0  machines   16  alive

Requirements (measurable part)
  [PASS] R2 complete and ordered: 0 readings missing, 0 out of order
  [PASS] R3 fresh: p95 latency < 5 s
  [PASS] detection: all faults found, at most 2 false alarms
```

All checks pass.
