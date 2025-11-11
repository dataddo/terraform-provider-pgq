---
page_title: "pgq_queue Resource"
description: |-
  Manages a pgq queue table in PostgreSQL.
---

# pgq_queue

Manages a pgq queue table in PostgreSQL. Supports both simple (non-partitioned) and partitioned queues with pg_partman integration.

## Example Usage

### Simple Queue

```terraform
resource "pgq_queue" "orders" {
  name   = "orders_queue"
  schema = "public"

  enable_partitioning = false
}
```

### Partitioned Queue with Daily Partitions

```terraform
resource "pgq_queue" "events" {
  name   = "events_queue"
  schema = "public"

  enable_partitioning  = true
  partition_interval   = "1 day"
  partition_premake    = 7
  retention_period     = "14 days"
  datetime_string      = "YYYYMMDD"
  optimize_constraint  = 30
  default_partition    = true
}
```

### Partitioned Queue with Weekly Partitions

```terraform
resource "pgq_queue" "analytics" {
  name   = "analytics_queue"
  schema = "public"

  enable_partitioning  = true
  partition_interval   = "1 week"
  partition_premake    = 4
  retention_period     = "90 days"
  datetime_string      = "IYYY_IW"
  optimize_constraint  = 12
}
```

### Partitioned Queue with Monthly Partitions

```terraform
resource "pgq_queue" "archive" {
  name   = "archive_queue"
  schema = "public"

  enable_partitioning  = true
  partition_interval   = "1 month"
  partition_premake    = 3
  retention_period     = "365 days"
  datetime_string      = "YYYY_MM"
  optimize_constraint  = 12
}
```

## Queue Schema

Each queue table includes the following columns:

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | NO | `gen_random_uuid()` | Primary key |
| `created_at` | TIMESTAMPTZ | NO | `CURRENT_TIMESTAMP` | Creation timestamp (partition key) |
| `started_at` | TIMESTAMPTZ | YES | | Processing start time |
| `locked_until` | TIMESTAMPTZ | YES | | Lock expiration |
| `scheduled_for` | TIMESTAMPTZ | YES | | Scheduled execution time |
| `processed_at` | TIMESTAMPTZ | YES | | Completion timestamp |
| `consumed_count` | INTEGER | NO | `0` | Consumption counter |
| `error_detail` | TEXT | YES | | Error information |
| `payload` | JSONB | NO | | Message payload |
| `metadata` | JSONB | NO | | Message metadata |

### Indexes

The following indexes are automatically created:

- `{queue_name}_created_at_idx` - Index on `created_at`
- `{queue_name}_processed_at_null_idx` - Partial index on `processed_at` WHERE `processed_at IS NULL`
- `{queue_name}_scheduled_for_idx` - Partial index on `scheduled_for` WHERE `processed_at IS NULL`
- `{queue_name}_metadata_idx` - GIN index on `metadata` WHERE `processed_at IS NULL`

## Argument Reference

### Required Arguments

- `name` (String) Name of the queue table. Changing this forces a new resource.

### Optional Arguments

- `schema` (String) PostgreSQL schema where the queue will be created. Default: `"public"`. Changing this forces a new resource.
- `enable_partitioning` (Boolean) Enable pg_partman partitioning for the queue. Default: `false`.

### Partitioning Arguments

The following arguments are only used when `enable_partitioning` is `true`:

- `partition_interval` (String) Time interval for partition creation. Default: `"1 day"`.
  - Examples: `"1 day"`, `"1 week"`, `"1 month"`, `"1 year"`
  - Must be a valid PostgreSQL interval expression

- `partition_premake` (Number) Number of partitions to create in advance. Default: `7`.
  - Recommended values:
    - Daily partitions: 7-14
    - Weekly partitions: 4-8
    - Monthly partitions: 3-6

- `retention_period` (String) How long to keep partitions before dropping them. Default: `"14 days"`.
  - Examples: `"14 days"`, `"30 days"`, `"90 days"`, `"1 year"`
  - Must be a valid PostgreSQL interval expression

- `datetime_string` (String) PostgreSQL datetime format string for partition naming. Default: `"YYYYMMDD"`.
  - Common formats:
    - `"YYYYMMDD"` - Daily: `queue_20231015`
    - `"YYYY_MM_DD"` - Daily with separators: `queue_2023_10_15`
    - `"IYYY_IW"` - ISO week: `queue_2023_42`
    - `"YYYY_MM"` - Monthly: `queue_2023_10`
    - `"YYYY_Q"` - Quarterly: `queue_2023_4`

- `optimize_constraint` (Number) Number of partitions to analyze for constraint optimization. Default: `30`.
  - Higher values improve query planning but increase maintenance time
  - Recommended: Set to cover your typical query range

- `default_partition` (Boolean) Create a default partition for rows that don't match any existing partition. Default: `true`.
  - Recommended to keep enabled to prevent insertion failures

## Attribute Reference

- `id` (String) Fully qualified name of the queue in the format `schema.name`

## Import

Existing queues can be imported using the fully qualified name:

```bash
terraform import pgq_queue.my_queue public.my_queue_name
```

Or for queues in other schemas:

```bash
terraform import pgq_queue.my_queue myschema.my_queue_name
```

## Partition Management

### Viewing Partitions

```sql
-- List all partitions for a queue
SELECT tablename
FROM pg_tables
WHERE tablename LIKE 'queue_name_%'
  AND schemaname = 'public'
ORDER BY tablename;

-- View partition configuration
SELECT *
FROM partman.part_config
WHERE parent_table = 'public.queue_name';

-- Check partition sizes
SELECT
  schemaname||'.'||tablename AS partition,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size,
  n_live_tup AS estimated_rows
FROM pg_stat_user_tables
WHERE tablename LIKE 'queue_name_%'
ORDER BY tablename DESC;
```

### Manual Maintenance

```sql
-- Run partition maintenance manually
SELECT partman.run_maintenance('public.queue_name');

-- Create additional partitions
SELECT partman.create_parent(
  'public.queue_name',
  'created_at',
  '1 day',
  p_premake := 14
);
```

### Monitoring

Set up monitoring for:
- Partition sizes
- Number of partitions
- Partition creation/deletion events
- pg_partman maintenance runs

Consider setting up pg_cron or system cron for automatic maintenance:

```sql
-- Using pg_cron
SELECT cron.schedule(
  'partition-maintenance',
  '0 3 * * *',
  'SELECT partman.run_maintenance_proc()'
);
```

## Best Practices

### Choosing Partition Intervals

- **High volume** (millions of messages/day): Daily or hourly partitions
- **Medium volume** (thousands of messages/day): Daily or weekly partitions
- **Low volume** (hundreds of messages/day): Weekly or monthly partitions

### Setting Retention

Consider:
- Business requirements
- Compliance and regulatory needs (GDPR, etc.)
- Storage costs
- Query performance needs
- Backup and recovery requirements

### Pre-creating Partitions

Set `partition_premake` to ensure partitions exist before needed:
- Daily partitions: 7-14 days ahead
- Weekly partitions: 4-8 weeks ahead
- Monthly partitions: 3-6 months ahead

### Performance Tips

1. **Index Strategy**: The default indexes cover most use cases, but consider your query patterns
2. **Partition Pruning**: Use `created_at` in WHERE clauses to enable partition pruning
3. **Constraint Optimization**: Set `optimize_constraint` to match your typical query range
4. **Maintenance Windows**: Schedule partition maintenance during low-traffic periods

## Troubleshooting

### pg_partman Extension Not Found

Ensure the extension is installed and enabled:

```sql
CREATE EXTENSION IF NOT EXISTS pg_partman SCHEMA partman;
```

### Permission Denied

Grant necessary privileges:

```sql
GRANT USAGE ON SCHEMA partman TO your_user;
GRANT ALL ON ALL TABLES IN SCHEMA partman TO your_user;
GRANT ALL ON ALL SEQUENCES IN SCHEMA partman TO your_user;
```

### Queue Already Exists

If you're trying to import an existing queue into Terraform, use the import command instead of creating a new resource.

### Partition Not Created

Check pg_partman maintenance:

```sql
-- Check last maintenance run
SELECT * FROM partman.part_config_sub ORDER BY last_run DESC LIMIT 10;

-- Manually trigger maintenance
SELECT partman.run_maintenance('public.your_queue');
```
