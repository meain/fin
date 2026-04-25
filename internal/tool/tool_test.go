package tool

import (
	"testing"
)

func TestDefs(t *testing.T) {
	tools := BuiltinTools()
	defs := Defs(tools)

	if len(defs) != len(tools) {
		t.Fatalf("Defs returned %d entries, want %d", len(defs), len(tools))
	}

	for i, def := range defs {
		if def.Name != tools[i].Name() {
			t.Errorf("defs[%d].Name = %q, want %q", i, def.Name, tools[i].Name())
		}
		if def.Description != tools[i].Description() {
			t.Errorf("defs[%d].Description mismatch", i)
		}
		if def.Parameters == nil {
			t.Errorf("defs[%d].Parameters is nil", i)
		}
	}
}

func TestFind(t *testing.T) {
	tools := BuiltinTools()

	for _, name := range []string{"read", "write", "edit", "shell"} {
		tl := Find(tools, name)
		if tl == nil {
			t.Errorf("Find(%q) returned nil", name)
		} else if tl.Name() != name {
			t.Errorf("Find(%q).Name() = %q", name, tl.Name())
		}
	}

	if tl := Find(tools, "nonexistent"); tl != nil {
		t.Errorf("Find(nonexistent) should be nil, got %v", tl)
	}
}

func TestBuiltinTools(t *testing.T) {
	tools := BuiltinTools()

	if len(tools) != 4 {
		t.Fatalf("BuiltinTools() returned %d tools, want 4", len(tools))
	}

	expected := []string{"read", "write", "edit", "shell"}
	for i, name := range expected {
		if tools[i].Name() != name {
			t.Errorf("tools[%d].Name() = %q, want %q", i, tools[i].Name(), name)
		}
	}
}
