# Exercise 2, Operation Data Plane with OLTPs

## Activity 2.1, PostgreSQL Analytical Queries (E-commerce)

### My CREATE TABLE for orders

```sql
DROP TABLE IF EXISTS orders;

CREATE TABLE orders (
  id SERIAL PRIMARY KEY,
  customer_name TEXT,
  product_category TEXT,
  quantity INTEGER,
  price_per_unit NUMERIC(10,2),
  order_date DATE,
  country TEXT
);

\COPY orders(customer_name,product_category,quantity,price_per_unit,order_date,country) FROM '/data/orders_1M.csv' DELIMITER ',' CSV HEADER;
```

I picked the types by just peeking at the first few rows of the CSV. TEXT for names, categories and country since they are plain words. INTEGER for quantity since it is a whole number. NUMERIC(10,2) for price_per_unit so money stays exact to 2 decimals and does not drift because of floating point weirdness. DATE for order_date so Postgres actually treats it like a date and not a string. SERIAL PRIMARY KEY on id so every row gets a unique number without me doing anything.

Load confirmed COPY 1000000.

### A. Which order has the highest price_per_unit?

```sql
SELECT * FROM orders ORDER BY price_per_unit DESC LIMIT 1;
```

Result: id 841292, Emma Brown, Automotive, quantity 3, price 2000.00, 2024-10-11, Italy. Someone bought a very expensive car part that day.

### B. Top 3 product categories by total quantity sold

```sql
SELECT product_category, SUM(quantity) AS total_quantity
FROM orders GROUP BY product_category
ORDER BY total_quantity DESC LIMIT 3;
```

Result: Health & Beauty 300842, Electronics 300804, Toys 300598.

### C. Total revenue per product category

```sql
SELECT product_category, SUM(price_per_unit * quantity) AS total_revenue
FROM orders GROUP BY product_category
ORDER BY total_revenue DESC;
```

Result from highest to lowest: Automotive 306,589,798.86 ; Electronics 241,525,009.45 ; Home & Garden 78,023,780.09 ; Sports 61,848,990.83 ; Health & Beauty 46,599,817.89 ; Office Supplies 38,276,061.64 ; Fashion 31,566,368.22 ; Toys 23,271,039.02 ; Grocery 15,268,355.66 ; Books 12,731,976.04.

### D. Top 5 customers by total spending

```sql
SELECT customer_name, SUM(price_per_unit * quantity) AS total_spent
FROM orders GROUP BY customer_name
ORDER BY total_spent DESC LIMIT 5;
```

Result: Carol Taylor 991,179.18 ; Nina Lopez 975,444.95 ; Daniel Jackson 959,344.48 ; Carol Lewis 947,708.57 ; Daniel Young 946,030.14.

Order counts for those five: Carol Taylor 1028, Nina Lopez 980, Daniel Jackson 1033, Carol Lewis 943, Daniel Young 973.

### E. What I noticed and why the data looks like this

First thing I noticed is that the top 5 customers all spent almost the same amount (around 950k to 991k), and each has about 1000 orders out of a million. In a real shop you would expect a few super loyal customers and lots of small ones, not five people who look like they were copy pasted from each other.

So I opened ecommerce/dataset_generator.py to see what is going on. The script just makes one million random orders. For every row it builds a customer name on the fly with random.choice(FIRST_NAMES) plus random.choice(LAST_NAMES). There is no list of real customers anywhere, it just invents a name for each row.

Since the first name and last name lists are small, the total number of possible full names is also small. If you spread 1,000,000 random names over such a small pool, every combination ends up showing roughly the same number of times, which here is about 1000. So the "top customers" are not really customers at all, they are just the name combos that got lucky in the random draw.

The totals are close for the same reason. Quantity is a random integer between 1 and 5, and price_per_unit is a random value in a fixed range per category, so the average revenue per order comes out about the same for everyone. Basically nobody is a VIP, the dice just rolled slightly nicer for a few names.

One more thing from C: Automotive is the top category by revenue even though it is not in the top 3 by quantity. Same reason, the generator gave Automotive the widest price range (50, 2000), so a few expensive orders (like Emma Brown's 2000.00 one) carry the whole category on their back.

## Activity 2.2, Why Is This Self-Join So Slow?

### Step 1, Measuring how it grows

I built the three smaller tables like the README said:

```sql
CREATE TABLE people_50k  AS SELECT * FROM people_big WHERE id <= 50000;
CREATE TABLE people_100k AS SELECT * FROM people_big WHERE id <= 100000;
CREATE TABLE people_200k AS SELECT * FROM people_big WHERE id <= 200000;
```

Then I ran the self join on each:

```sql
SELECT COUNT(*) FROM people_50k  p1 JOIN people_50k  p2 ON p1.country = p2.country;
SELECT COUNT(*) FROM people_100k p1 JOIN people_100k p2 ON p1.country = p2.country;
SELECT COUNT(*) FROM people_200k p1 JOIN people_200k p2 ON p1.country = p2.country;
```

| rows in table | join result (COUNT(*)) | time |
|---|---|---|
| 50,000 | 27,501,822 | 927 ms |
| 100,000 | 109,946,508 | 3,370 ms |
| 200,000 | 439,395,606 | 14,598 ms |

Every time I double the input, both the result and the time grow about 4 times. That is the classic n squared pattern. If a country has k people, pairing them within that country gives k times k pairs, so when k doubles, pairs go up by 4.

My prediction for people_big (1,000,000 rows): going from 200k to 1M is a factor of 5, so result and time should grow by about 25.
Predicted result: 439,395,606 * 25 which is about 11 billion pairs.
Predicted time: 14.60 s * 25 which is about 365 s, so around 6 minutes. Enough time to make coffee and regret your query.

### Step 2, Does an index help?

```sql
CREATE INDEX idx_people_100k_country ON people_100k(country);
ANALYZE people_100k;
SELECT COUNT(*) FROM people_100k p1 JOIN people_100k p2 ON p1.country = p2.country;
```

Same count 109,946,508, time went from 3,370 ms to 3,284 ms. Basically nothing.

An index helps when the database has to find a few specific rows quickly. Here it still has to spit out every single pair of people in the same country, about 110 million rows. Even a perfect index cannot skip that work, so it does not get faster.

### Step 3, Rewriting it without a join

The task only wants the count of pairs, not the pairs themselves. If one country has k people, it contributes k times k pairs. So instead of actually making those pairs, I can just count how many people are in each country and sum the squares:

```sql
SELECT SUM(cnt * cnt)
FROM (SELECT COUNT(*) AS cnt FROM people_100k GROUP BY country) s;
```

On people_100k: same answer 109,946,508 but in 12.966 ms. About 250 times faster than the join.

On the full people_big (1,000,000 rows): 10,983,941,260 (around 11 billion, matches my prediction) in 148 ms. The join would have taken around 6 minutes on the same data, so the rewrite is roughly 2,500 times faster. Not bad for a bit of math.

### Step 4, Discussion

What the rewrite tells me about adding hardware or an index.
The join is slow because the output itself is huge, around 11 billion rows on the full table. An index does not help, since the DB still has to produce all those rows anyway (Step 2 showed this). A bigger machine helps a bit, but only by a constant factor. The problem grows n squared, so hardware loses the race as soon as the data gets a bit bigger. Step 3 is the real win because it changes the algorithm: count people per country and square. That is why 1M rows runs in 148 ms instead of 6 minutes. For this kind of problem a smart query beats a bigger server.

What if the business actually needed the pairs themselves, not just the count.
Then there is no clever shortcut, 11 billion rows have to be produced, and one Postgres node is not the place. The usual move is a distributed engine like Spark, Trino, BigQuery, or a Postgres sharded by country. Each country can be processed on its own, so adding workers roughly divides the runtime. I would also push back first, because 11 billion raw pairs is almost always a middle step towards some aggregate. If the aggregate can be done per country (like in Step 3), do that. If pairs really must be stored, write them to columnar files like Parquet partitioned by country so later reads stay cheap.

Limits of an OLTP database in a large scale cloud environment.
Postgres is great at many small transactions with strong consistency, but this workload is a mismatch. It stores data row by row, so touching a couple of columns across millions of rows is slow. Columnar engines (DuckDB, ClickHouse, BigQuery, Snowflake) are usually 10 to 100 times faster for this kind of query. It also runs on one node, so it cannot use the cloud's horizontal capacity to split a big join across machines. On top of that, a six minute query hogs CPU, memory and I/O that everyone else needs, so on a shared production DB it slows the whole room down. The modern setup keeps OLTP for the app and sends analytics to a separate OLAP engine on columnar storage, usually via CDC or ETL, so each side can scale on its own.

My takeaway: for this exercise, a smarter query made the biggest difference (around 2,500 times faster). After that comes using the right kind of engine. Throwing hardware or an index at the original join would have been the obvious reflex, and the experiment showed it would have been a waste of money.
