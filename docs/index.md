---
page_title: "PGQ Provider"
description: |-
  Terraform provider for managing pgq (PostgreSQL queue) resources.
---

# PGQ Provider

The PGQ provider allows you to manage pgq queue tables in PostgreSQL using Terraform. It provides native resource types for creating and managing both simple and partitioned queues with pg_partman integration.

## Features

- **Native Resource Management**: Manage queues as native Terraform resources
- **Simple Queues**: Create non-partitioned queue tables
- **Partitioned Queues**: Full pg_partman integration with automatic partition management
- **Direct PostgreSQL Connection**: Uses pgx library for direct PostgreSQL connectivity
- **Environment Variable Support**: Configure using standard PostgreSQL environment variables

## Example Usage

```terraform
# Configure the provider
provider "pgq" {
  host     = "localhost"
  port     = 5432
  database = "mydb"
  username = "postgres"
  password = var.db_password
}

# Create a simple queue
resource "pgq_queue" "orders" {
  name   = "orders_queue"
  schema = "public"

  enable_partitioning = false
}

# Create a partitioned queue
resource "pgq_queue" "events" {
  name   = "events_queue"
  schema = "public"

  enable_partitioning  = true
  partition_interval   = "1 day"
  partition_premake    = 7
  retention_period     = "14 days"
}
```

## Environment Variables

The provider supports standard PostgreSQL environment variables:

- `PGHOST` - PostgreSQL server hostname
- `PGPORT` - PostgreSQL server port
- `PGDATABASE` - PostgreSQL database name
- `PGUSER` - PostgreSQL username
- `PGPASSWORD` - PostgreSQL password
- `PGSSLMODE` - SSL mode (disable, require, verify-ca, verify-full)

When using environment variables, the provider configuration can be simplified:

```terraform
provider "pgq" {}
```

## Schema

### Required

None. All configuration can be provided via environment variables.

### Optional

- `host` (String) PostgreSQL server hostname. Can be set via `PGHOST` environment variable.
- `port` (Number) PostgreSQL server port. Default: `5432`. Can be set via `PGPORT` environment variable.
- `database` (String) PostgreSQL database name. Can be set via `PGDATABASE` environment variable.
- `username` (String) PostgreSQL username. Can be set via `PGUSER` environment variable.
- `password` (String, Sensitive) PostgreSQL password. Can be set via `PGPASSWORD` environment variable.
- `sslmode` (String) PostgreSQL SSL mode. Default: `prefer`. Can be set via `PGSSLMODE` environment variable.
  - Valid values: `disable`, `require`, `verify-ca`, `verify-full`

## Prerequisites

- PostgreSQL 12 or later
- For partitioned queues: pg_partman extension must be installed and enabled

To enable pg_partman:

```sql
CREATE EXTENSION IF NOT EXISTS pg_partman SCHEMA partman;
```
