package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestQueueModel(t *testing.T) {
	m := queueModel{
		Name:               types.StringValue("test"),
		Schema:             types.StringValue("public"),
		EnablePartitioning: types.BoolValue(true),
		PartitionInterval:  types.StringValue("1 day"),
		PartitionPremake:   types.Int64Value(7),
	}

	if m.Name.ValueString() != "test" {
		t.Errorf("Name = %q, want %q", m.Name.ValueString(), "test")
	}

	if !m.EnablePartitioning.ValueBool() {
		t.Error("EnablePartitioning should be true")
	}
}
