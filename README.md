# Terraform Provider for PGQ

A Terraform provider for managing [pgq](https://github.com/dataddo/pgq) queue tables in PostgreSQL. This provider allows you to create, manage, and delete pgq queue tables as native Terraform resources, with full support for both simple and partitioned queues using [pg_partman](https://github.com/pgpartman/pg_partman).

## Features

- **Native Terraform Resources**: Manage queues as `pgq_queue` resources
- **Simple Queues**: Create non-partitioned queue tables for smaller workloads
- **Partitioned Queues**: Full pg_partman integration for automatic partition management
- **Direct PostgreSQL Connection**: Uses pgx library directly, no postgresql provider dependency
- **Configurable Retention**: Automatic cleanup of old partitions
- **Environment Variable Support**: Configure connection via standard PostgreSQL environment variables
- **Full CRUD Support**: Create, Read, Update, and Delete operations
- **Import Support**: Import existing queues into Terraform state

## Requirements

- Terraform 1.0+
- PostgreSQL 12+
- Go 1.23+ (for building from source)
- pg_partman extension (for partitioned queues)

## Installation

### Using the Provider

Add the provider to your Terraform configuration:

```hcl
terraform {
  required_providers {
    pgq = {
      source  = "dataddo/pgq"
      version = "~> 0.1"
    }
  }
}
```

### Building from Source

```bash
git clone https://github.com/dataddo/terraform-provider-pgq.git
cd terraform-provider-pgq
make install
```

This will build and install the provider to your local Terraform plugin directory.

## Usage

### Provider Configuration

```hcl
provider "pgq" {
  host     = "localhost"
  port     = 5432
  database = "mydb"
  username = "postgres"
  password = var.db_password
  sslmode  = "prefer"
}
```

Or use environment variables (recommended):

```bash
export PGHOST=localhost
export PGPORT=5432
export PGDATABASE=mydb
export PGUSER=postgres
export PGPASSWORD=yourpassword
export PGSSLMODE=prefer
```

```hcl
provider "pgq" {}
```

### Simple Queue

Create a non-partitioned queue:

```hcl
resource "pgq_queue" "orders" {
  name   = "orders_queue"
  schema = "public"

  enable_partitioning = false
}
```

### Partitioned Queue

Create a partitioned queue with pg_partman:

```hcl
resource "pgq_queue" "events" {
  name   = "events_queue"
  schema = "public"

  # Enable partitioning
  enable_partitioning = true

  # Partition configuration
  partition_interval   = "1 day"      # Create daily partitions
  partition_premake    = 7            # Create 7 days ahead
  retention_period     = "14 days"    # Keep 14 days of data
  datetime_string      = "YYYYMMDD"   # Partition naming
  optimize_constraint  = 30           # Optimize last 30 partitions
  default_partition    = true         # Create default partition
}
```

## Queue Schema

Each queue table includes the following columns:

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key, auto-generated |
| `created_at` | TIMESTAMPTZ | Creation timestamp (partition key for partitioned queues) |
| `started_at` | TIMESTAMPTZ | When message processing started |
| `locked_until` | TIMESTAMPTZ | Lock expiration timestamp |
| `scheduled_for` | TIMESTAMPTZ | When message should be processed |
| `processed_at` | TIMESTAMPTZ | When message was completed |
| `consumed_count` | INTEGER | Number of times message was consumed |
| `error_detail` | TEXT | Error information if processing failed |
| `payload` | JSONB | Message payload |
| `metadata` | JSONB | Message metadata |

### Indexes

Automatically created indexes:

- `created_at_idx`: Index on `created_at`
- `processed_at_null_idx`: Partial index for unprocessed messages
- `scheduled_for_idx`: Index for scheduled messages
- `metadata_idx`: GIN index on metadata JSONB

## Resource: pgq_queue

### Arguments

#### Required

- `name` (String) - Name of the queue table. Forces replacement if changed.

#### Optional

- `schema` (String) - PostgreSQL schema. Default: `"public"`. Forces replacement if changed.
- `enable_partitioning` (Boolean) - Enable pg_partman partitioning. Default: `false`.

#### Partitioning Configuration (only when `enable_partitioning = true`)

- `partition_interval` (String) - Time interval for partitions. Default: `"1 day"`.
  - Examples: `"1 day"`, `"1 week"`, `"1 month"`, `"1 year"`
- `partition_premake` (Number) - Number of partitions to create ahead. Default: `7`.
- `retention_period` (String) - How long to keep partitions. Default: `"14 days"`.
  - Examples: `"14 days"`, `"30 days"`, `"3 months"`, `"1 year"`
- `datetime_string` (String) - PostgreSQL datetime format for partition naming. Default: `"YYYYMMDD"`.
  - Examples: `"YYYYMMDD"`, `"YYYY_MM_DD"`, `"IYYY_IW"` (ISO week), `"YYYY_MM"`
- `optimize_constraint` (Number) - Number of partitions to optimize. Default: `30`.
- `default_partition` (Boolean) - Create default partition for unmatched rows. Default: `true`.

### Attributes

- `id` (String) - Fully qualified name of the queue (`schema.name`)

### Import

Import existing queues using the fully qualified name:

```bash
terraform import pgq_queue.my_queue public.my_queue_name
```

## Examples

See the [examples](./examples/) directory for complete examples:

- [Simple Queues](./examples/simple/) - Non-partitioned queues
- [Partitioned Queues](./examples/partitioned/) - Partitioned queues with various configurations

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Installing Locally

```bash
make install
```

### Running Examples

```bash
# Simple example
make example-simple

# Partitioned example
make example-partitioned
```

### Code Quality

```bash
# Format code
make fmt

# Run vet
make vet

# Run linter (requires golangci-lint)
make lint

# All checks
make dev
```

## License

This project is part of the pgq ecosystem and follows the same license as pgq.

## Support

For issues, questions, or contributions, please visit:
- [GitHub Issues](https://github.com/dataddo/terraform-provider-pgq/issues)
- [pgq Repository](https://github.com/dataddo/pgq)
