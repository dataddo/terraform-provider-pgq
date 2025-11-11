package provider

import (
	"context"
	"fmt"

	"github.com/dataddo/terraform-provider-pgq/internal/pgq"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = (*queueResource)(nil)
	_ resource.ResourceWithConfigure   = (*queueResource)(nil)
	_ resource.ResourceWithImportState = (*queueResource)(nil)
)

type (
	queueResource struct {
		mgr *pgq.Manager
	}

	queueModel struct {
		ID                 types.String `tfsdk:"id"`
		Name               types.String `tfsdk:"name"`
		Schema             types.String `tfsdk:"schema"`
		EnablePartitioning types.Bool   `tfsdk:"enable_partitioning"`
		PartitionInterval  types.String `tfsdk:"partition_interval"`
		PartitionPremake   types.Int64  `tfsdk:"partition_premake"`
		RetentionPeriod    types.String `tfsdk:"retention_period"`
		DatetimeString     types.String `tfsdk:"datetime_string"`
		OptimizeConstraint types.Int64  `tfsdk:"optimize_constraint"`
		DefaultPartition   types.Bool   `tfsdk:"default_partition"`
		CustomIndexes      types.Set    `tfsdk:"custom_index"`
	}

	customIndexModel struct {
		Name    types.String `tfsdk:"name"`
		Columns types.List   `tfsdk:"columns"`
		Type    types.String `tfsdk:"type"`
		Where   types.String `tfsdk:"where"`
	}
)

func NewQueueResource() resource.Resource {
	return &queueResource{}
}

func convertCustomIndexes(ctx context.Context, models []customIndexModel) ([]pgq.CustomIndex, diag.Diagnostics) {
	var diags diag.Diagnostics
	indexes := make([]pgq.CustomIndex, 0, len(models))

	for _, m := range models {
		var columns []string
		if d := m.Columns.ElementsAs(ctx, &columns, false); d.HasError() {
			diags.Append(d...)
			continue
		}

		idx := pgq.CustomIndex{
			Name:    m.Name.ValueString(),
			Columns: columns,
			Type:    m.Type.ValueString(),
			Where:   m.Where.ValueString(),
		}
		indexes = append(indexes, idx)
	}

	return indexes, diags
}

func convertToCustomIndexModels(ctx context.Context, indexes []pgq.CustomIndex) ([]customIndexModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	models := make([]customIndexModel, 0, len(indexes))

	for _, idx := range indexes {
		cols, d := types.ListValueFrom(ctx, types.StringType, idx.Columns)
		if d.HasError() {
			diags.Append(d...)
			continue
		}

		m := customIndexModel{
			Name:    types.StringValue(idx.Name),
			Columns: cols,
			Type:    types.StringValue(idx.Type),
		}

		if idx.Where != "" {
			m.Where = types.StringValue(idx.Where)
		} else {
			m.Where = types.StringNull()
		}

		models = append(models, m)
	}

	return models, diags
}

func (r *queueResource) createCustomIndexesInTransaction(ctx context.Context, schema pgq.SchemaName, name pgq.QueueName, indexes []pgq.CustomIndex) error {
	tx, err := r.mgr.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := r.mgr.CreateCustomIndexes(ctx, tx, schema, name, indexes); err != nil {
		return fmt.Errorf("failed to create custom indexes: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit custom indexes: %w", err)
	}

	return nil
}

func customIndexObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":    types.StringType,
			"columns": types.ListType{ElemType: types.StringType},
			"type":    types.StringType,
			"where":   types.StringType,
		},
	}
}

func indexDefinitionEqual(ctx context.Context, a, b customIndexModel) (bool, error) {
	if a.Name.ValueString() != b.Name.ValueString() {
		return false, nil
	}
	if a.Type.ValueString() != b.Type.ValueString() {
		return false, nil
	}
	if a.Where.ValueString() != b.Where.ValueString() {
		return false, nil
	}

	var aCols, bCols []string
	if diags := a.Columns.ElementsAs(ctx, &aCols, false); diags.HasError() {
		return false, fmt.Errorf("failed to extract columns from index a: %v", diags)
	}
	if diags := b.Columns.ElementsAs(ctx, &bCols, false); diags.HasError() {
		return false, fmt.Errorf("failed to extract columns from index b: %v", diags)
	}

	if len(aCols) != len(bCols) {
		return false, nil
	}

	for i := range aCols {
		if aCols[i] != bCols[i] {
			return false, nil
		}
	}

	return true, nil
}

func (r *queueResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_queue"
}

func (r *queueResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "pgq queue table",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Fully qualified name (schema.name)",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description:   "Queue name",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"schema": schema.StringAttribute{
				Description:   "PostgreSQL schema",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("public"),
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enable_partitioning": schema.BoolAttribute{
				Description:   "Enable pg_partman partitioning",
				Optional:      true,
				Computed:      true,
				Default:       booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"partition_interval": schema.StringAttribute{
				Description: "Partition interval (e.g. '1 day', '1 week')",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("1 day"),
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"partition_premake": schema.Int64Attribute{
				Description: "Partitions to create ahead",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(7),
			},
			"retention_period": schema.StringAttribute{
				Description: "How long to keep partitions (e.g. '14 days')",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("14 days"),
			},
			"datetime_string": schema.StringAttribute{
				Description: "Partition naming format (e.g. 'YYYYMMDD')",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("YYYYMMDD"),
			},
			"optimize_constraint": schema.Int64Attribute{
				Description: "Partitions to optimize",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(30),
			},
			"default_partition": schema.BoolAttribute{
				Description: "Create default partition",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
		},
		Blocks: map[string]schema.Block{
			"custom_index": schema.SetNestedBlock{
				Description: "Custom indexes to create on the queue table",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Index name (auto-generated if not provided)",
							Optional:    true,
							Computed:    true,
						},
						"columns": schema.ListAttribute{
							Description: "Column expressions (e.g. 'created_at', '(payload->>''user_id'')')",
							Required:    true,
							ElementType: types.StringType,
						},
						"type": schema.StringAttribute{
							Description: "Index type",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("btree"),
							Validators: []validator.String{
								stringvalidator.OneOf("btree", "gin", "gist", "hash", "brin"),
							},
						},
						"where": schema.StringAttribute{
							Description: "Partial index WHERE clause",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (r *queueResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	mgr, ok := req.ProviderData.(*pgq.Manager)
	if !ok {
		resp.Diagnostics.AddError("Unexpected type", fmt.Sprintf("Expected *pgq.Manager, got %T", req.ProviderData))
		return
	}

	r.mgr = mgr
}

func (r *queueResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan queueModel
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	schema := pgq.SchemaName(plan.Schema.ValueString())
	name := pgq.QueueName(plan.Name.ValueString())

	tflog.Debug(ctx, "creating queue", map[string]any{
		"fqn":         string(pgq.MakeFQN(schema, name)),
		"partitioned": plan.EnablePartitioning.ValueBool(),
	})

	if plan.EnablePartitioning.ValueBool() {
		cfg := &pgq.PartitionConfig{
			Interval:           plan.PartitionInterval.ValueString(),
			Premake:            int(plan.PartitionPremake.ValueInt64()),
			Retention:          plan.RetentionPeriod.ValueString(),
			DatetimeString:     plan.DatetimeString.ValueString(),
			OptimizeConstraint: int(plan.OptimizeConstraint.ValueInt64()),
			DefaultPartition:   plan.DefaultPartition.ValueBool(),
		}

		if err := r.mgr.CreatePartitioned(ctx, schema, name, cfg); err != nil {
			resp.Diagnostics.AddError("Failed to create partitioned queue", err.Error())
			return
		}
	} else {
		if err := r.mgr.CreateSimple(ctx, schema, name); err != nil {
			resp.Diagnostics.AddError("Failed to create queue", err.Error())
			return
		}
	}

	if !plan.CustomIndexes.IsNull() && !plan.CustomIndexes.IsUnknown() {
		var customIndexes []customIndexModel
		if diags := plan.CustomIndexes.ElementsAs(ctx, &customIndexes, false); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		indexes, diags := convertCustomIndexes(ctx, customIndexes)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		if err := r.createCustomIndexesInTransaction(ctx, schema, name, indexes); err != nil {
			resp.Diagnostics.AddError("Failed to create custom indexes", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(string(pgq.MakeFQN(schema, name)))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *queueResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state queueModel
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	schema := pgq.SchemaName(state.Schema.ValueString())
	name := pgq.QueueName(state.Name.ValueString())

	q, err := r.mgr.Get(ctx, schema, name)
	if err != nil {
		if _, ok := err.(*pgq.QueueNotFoundError); ok {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read queue", err.Error())
		return
	}

	state.EnablePartitioning = types.BoolValue(q.Partitioned)

	if q.Partitioned {
		cfg, err := r.mgr.GetPartitionConfig(ctx, schema, name)
		if err != nil {
			tflog.Warn(ctx, "failed to read partition config", map[string]any{"error": err})
		} else {
			state.PartitionInterval = types.StringValue(cfg.Interval)
			state.PartitionPremake = types.Int64Value(int64(cfg.Premake))
			state.RetentionPeriod = types.StringValue(cfg.Retention)
			state.DatetimeString = types.StringValue(cfg.DatetimeString)
			state.OptimizeConstraint = types.Int64Value(int64(cfg.OptimizeConstraint))
			state.DefaultPartition = types.BoolValue(cfg.DefaultPartition)
		}
	}

	customIndexes, err := r.mgr.GetCustomIndexes(ctx, schema, name)
	if err != nil {
		tflog.Warn(ctx, "failed to read custom indexes", map[string]any{"error": err})
	} else {
		models, diags := convertToCustomIndexModels(ctx, customIndexes)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		if len(models) > 0 {
			set, diags := types.SetValueFrom(ctx, customIndexObjectType(), models)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.CustomIndexes = set
		} else {
			state.CustomIndexes = types.SetNull(customIndexObjectType())
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *queueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state queueModel
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	schema := pgq.SchemaName(plan.Schema.ValueString())
	name := pgq.QueueName(plan.Name.ValueString())

	if state.EnablePartitioning.ValueBool() && plan.EnablePartitioning.ValueBool() {
		cfg := &pgq.PartitionConfig{
			Interval:           plan.PartitionInterval.ValueString(),
			Premake:            int(plan.PartitionPremake.ValueInt64()),
			Retention:          plan.RetentionPeriod.ValueString(),
			DatetimeString:     plan.DatetimeString.ValueString(),
			OptimizeConstraint: int(plan.OptimizeConstraint.ValueInt64()),
			DefaultPartition:   plan.DefaultPartition.ValueBool(),
		}

		if err := r.mgr.UpdatePartitionConfig(ctx, schema, name, cfg); err != nil {
			resp.Diagnostics.AddError("Failed to update partition config", err.Error())
			return
		}
	}

	if !plan.CustomIndexes.Equal(state.CustomIndexes) {
		var stateIndexes, planIndexes []customIndexModel

		if !state.CustomIndexes.IsNull() && !state.CustomIndexes.IsUnknown() {
			if diags := state.CustomIndexes.ElementsAs(ctx, &stateIndexes, false); diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		}

		if !plan.CustomIndexes.IsNull() && !plan.CustomIndexes.IsUnknown() {
			if diags := plan.CustomIndexes.ElementsAs(ctx, &planIndexes, false); diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		}

		stateMap := make(map[string]customIndexModel)
		for _, idx := range stateIndexes {
			stateMap[idx.Name.ValueString()] = idx
		}

		planMap := make(map[string]customIndexModel)
		for _, idx := range planIndexes {
			planMap[idx.Name.ValueString()] = idx
		}

		var toDrop []string
		for stateName, stateIdx := range stateMap {
			planIdx, existsInPlan := planMap[stateName]
			if !existsInPlan {
				toDrop = append(toDrop, stateName)
			} else {
				equal, err := indexDefinitionEqual(ctx, stateIdx, planIdx)
				if err != nil {
					resp.Diagnostics.AddError("Failed to compare index definitions", err.Error())
					return
				}
				if !equal {
					toDrop = append(toDrop, stateName)
				}
			}
		}

		if len(toDrop) > 0 {
			if err := r.mgr.DropCustomIndexes(ctx, schema, name, toDrop); err != nil {
				resp.Diagnostics.AddError("Failed to drop custom indexes", err.Error())
				return
			}
		}

		var toCreate []customIndexModel
		for planName, planIdx := range planMap {
			stateIdx, existsInState := stateMap[planName]
			if !existsInState {
				toCreate = append(toCreate, planIdx)
			} else {
				equal, err := indexDefinitionEqual(ctx, stateIdx, planIdx)
				if err != nil {
					resp.Diagnostics.AddError("Failed to compare index definitions", err.Error())
					return
				}
				if !equal {
					toCreate = append(toCreate, planIdx)
				}
			}
		}

		if len(toCreate) > 0 {
			indexes, diags := convertCustomIndexes(ctx, toCreate)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}

			if err := r.createCustomIndexesInTransaction(ctx, schema, name, indexes); err != nil {
				resp.Diagnostics.AddError("Failed to create custom indexes", err.Error())
				return
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *queueResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state queueModel
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	schema := pgq.SchemaName(state.Schema.ValueString())
	name := pgq.QueueName(state.Name.ValueString())

	if state.EnablePartitioning.ValueBool() {
		if err := r.mgr.RemovePartmanConfig(ctx, schema, name); err != nil {
			tflog.Warn(ctx, "failed to remove partman config", map[string]any{"error": err})
		}
	}

	if err := r.mgr.Drop(ctx, schema, name); err != nil {
		resp.Diagnostics.AddError("Failed to drop queue", err.Error())
		return
	}
}

func (r *queueResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
