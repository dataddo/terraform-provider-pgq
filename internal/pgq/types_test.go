package pgq

import "testing"

func TestQueueNameValid(t *testing.T) {
	tests := []struct {
		name  QueueName
		valid bool
	}{
		{"valid_queue", true},
		{"queue123", true},
		{"_queue", true},
		{"", false},
		{"123queue", false},
		{QueueName(string(make([]byte, 64))), false},
	}

	for _, tt := range tests {
		if got := tt.name.Valid(); got != tt.valid {
			t.Errorf("QueueName(%q).Valid() = %v, want %v", tt.name, got, tt.valid)
		}
	}
}

func TestFQN(t *testing.T) {
	schema := SchemaName("public")
	name := QueueName("test_queue")

	fqn := MakeFQN(schema, name)
	if fqn != "public.test_queue" {
		t.Errorf("MakeFQN() = %q, want %q", fqn, "public.test_queue")
	}

	gotSchema, gotName, err := fqn.Split()
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	if gotSchema != schema {
		t.Errorf("Split() schema = %q, want %q", gotSchema, schema)
	}
	if gotName != name {
		t.Errorf("Split() name = %q, want %q", gotName, name)
	}
}

func TestFQNSplitInvalid(t *testing.T) {
	tests := []FQN{
		"invalid",
		"",
	}

	for _, fqn := range tests {
		_, _, err := fqn.Split()
		if err == nil {
			t.Errorf("Split(%q) expected error, got nil", fqn)
		}
	}
}

func TestQueueFQN(t *testing.T) {
	q := &Queue{
		Schema: "myschema",
		Name:   "myqueue",
	}

	if got := q.FQN(); got != "myschema.myqueue" {
		t.Errorf("Queue.FQN() = %q, want %q", got, "myschema.myqueue")
	}
}

func TestTemplateName(t *testing.T) {
	q := &Queue{
		Schema: "public",
		Name:   "test",
	}

	tmpl := q.TemplateName()
	if tmpl != "test_template" {
		t.Errorf("TemplateName() = %q, want %q", tmpl, "test_template")
	}

	tmplFQN := q.TemplateFQN()
	if tmplFQN != "public.test_template" {
		t.Errorf("TemplateFQN() = %q, want %q", tmplFQN, "public.test_template")
	}
}
