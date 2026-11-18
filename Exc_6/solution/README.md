# Exercise 6: Card Sentinel

## Part 1: My decision

For this scenario I would choose log-based CDC with Debezium. The payment path belongs to another team and is certified, so I am not allowed to touch it, which rules out dual write and the outbox. Polling is also not a good option, because every agent would keep querying the database and it can miss payments that commit out of id order. With CDC, Debezium simply reads the write-ahead log of PostgreSQL, so nothing in the application changes, there are no extra queries, and every committed payment arrives in the right order. The price is the replication slot, because PostgreSQL keeps all the WAL that the connector has not read yet, and if the connector stays down for a long time the disk can fill up and stop the whole database. To catch this early, I would monitor the connector status and how much WAL the slot is holding back, set an alert on disk usage, and use max_slot_wal_keep_size as a safety limit. If my own team owned the payment path, I would go with the outbox instead, since it is just as reliable and gives us control over the events.

## Part 2: The three runs

Setup for every run:

- 1,000 payments per second
- 3 velocity agents and 1 anomaly agent
- scoreboard read after about 155 seconds

### Run 1: Debezium's defaults

**My prediction**

- The three velocity agents share the work.
- R4 passes.

**Scoreboard (velocity)**

```
R3  frauds caught          2/8 (25%)        FAIL
R4  alert latency p50/p95  65.9 s / 88.1 s  FAIL
```

**What happened and why**

- The topic had only one partition, and one partition can only be read by one agent in a group, so vel-1 did all the work while vel-2 and vel-3 sat idle.
- vel-1 handled only about 370 payments per second, so it fell further and further behind.
- Alerts came around 88 seconds late, and most frauds were still stuck in the backlog.

### Run 2: Six partitions

**My prediction**

- All three agents now work.
- R3 and R4 both pass.

**Scoreboard (velocity)**

```
R3  frauds caught  0/8 (0%)                    FAIL
R4  no alerts, worst lag of the agents 1.1 s   PASS
```

**What happened and why**

- Each agent got two partitions and kept up easily, so R4 passed.
- The key was still the payment id, so the 12 test payments of one stolen card were spread over all three agents, and each one saw only about 4 of them, below the limit of 8.
- My prediction was wrong: more partitions fixed the speed but broke the counting.

### Run 3: The cardholder as the key

**My prediction**

- The key changes to the user id.
- The velocity model finally catches the frauds.

**Scoreboard (velocity)**

```
R3  frauds caught          8/8 (100%)     PASS
R4  alert latency p50/p95  1.0 s / 1.1 s  PASS
```

**What happened and why**

- The key in the Kafka UI was now the user id, so all payments of one cardholder went to the same partition and the same agent, in the right order.
- That agent saw all 12 test payments and raised the alert at the 8th one, while all three agents still kept up.
- This is my final setup. The anomaly model passed in all three runs, with 8 out of 8 frauds and about 1 second of delay.

## Closing answer

**The two defaults I had to change**

- Only one partition
- The payment id as the key

**What would have happened in production**

- With one partition, only one velocity agent can work, so the backlog keeps growing and alerts arrive minutes late, no matter how many agents you start.
- With more partitions but the payment id as the key, everything looks healthy, with no lag and no errors, but the model quietly misses almost every card testing attack. This is the more dangerous one.

**How the team could have noticed**

- Watching the consumer lag would show the first problem.
- Checking the partition count and the key in the Kafka UI would show the second one.
- Comparing the caught frauds with later chargebacks, or sending in test frauds on purpose, would also reveal it.

My final connector is in fraud-connector.json in this folder.
