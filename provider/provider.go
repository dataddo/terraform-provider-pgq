package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/dataddo/terraform-provider-pgq/internal/pgq"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ provider.Provider = (*pgqProvider)(nil)

type (
	pgqProvider struct {
		version string
	}

	config struct {
		Host     types.String `tfsdk:"host"`
		Port     types.Int64  `tfsdk:"port"`
		Database types.String `tfsdk:"database"`
		Username types.String `tfsdk:"username"`
		Password types.String `tfsdk:"password"`
		SSLMode  types.String `tfsdk:"sslmode"`
	}
)

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &pgqProvider{version: version}
	}
}

func (p *pgqProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pgq"
	resp.Version = p.version
}

func (p *pgqProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage pgq queues in PostgreSQL",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "PostgreSQL hostname (env: PGHOST)",
				Optional:    true,
			},
			"port": schema.Int64Attribute{
				Description: "PostgreSQL port (env: PGPORT, default: 5432)",
				Optional:    true,
			},
			"database": schema.StringAttribute{
				Description: "Database name (env: PGDATABASE)",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Username (env: PGUSER)",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password (env: PGPASSWORD)",
				Optional:    true,
				Sensitive:   true,
			},
			"sslmode": schema.StringAttribute{
				Description: "SSL mode: disable, require, verify-ca, verify-full (env: PGSSLMODE, default: prefer)",
				Optional:    true,
			},
		},
	}
}

func (p *pgqProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg config
	if diags := req.Config.Get(ctx, &cfg); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	connStr := p.buildConnString(cfg)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		resp.Diagnostics.AddError("Connection pool creation failed", err.Error())
		return
	}

	if err := pool.Ping(ctx); err != nil {
		resp.Diagnostics.AddError("PostgreSQL connection failed", err.Error())
		return
	}

	mgr := pgq.NewManager(pool)
	resp.DataSourceData = mgr
	resp.ResourceData = mgr
}

func (p *pgqProvider) buildConnString(cfg config) string {
	host := valOrEnv(cfg.Host, "PGHOST", "localhost")
	port := portOrEnv(cfg.Port, "PGPORT", 5432)
	db := valOrEnv(cfg.Database, "PGDATABASE", "postgres")
	user := valOrEnv(cfg.Username, "PGUSER", "postgres")
	pass := valOrEnv(cfg.Password, "PGPASSWORD", "")
	ssl := valOrEnv(cfg.SSLMode, "PGSSLMODE", "prefer")

	return fmt.Sprintf(
		"host=%s port=%d database=%s user=%s password=%s sslmode=%s",
		host, port, db, user, pass, ssl,
	)
}

func valOrEnv(val types.String, env, def string) string {
	if !val.IsNull() && !val.IsUnknown() {
		return val.ValueString()
	}
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

func portOrEnv(val types.Int64, env string, def int64) int64 {
	if !val.IsNull() && !val.IsUnknown() {
		return val.ValueInt64()
	}
	if v := os.Getenv(env); v != "" {
		if port, err := strconv.ParseInt(v, 10, 64); err == nil {
			return port
		}
	}
	return def
}

func (p *pgqProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *pgqProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewQueueResource,
	}
}
