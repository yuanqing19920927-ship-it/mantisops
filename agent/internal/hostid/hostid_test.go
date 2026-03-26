package hostid

import "testing"

func TestGet_WithConfigID(t *testing.T) {
	id := Get("manual-id")
	if id != "manual-id" {
		t.Fatalf("expected manual-id, got %s", id)
	}
}

func TestGenerate_ReturnsNonEmpty(t *testing.T) {
	id := generate()
	if id == "" {
		t.Fatal("generate returned empty")
	}
	if len(id) < 8 {
		t.Fatalf("id too short: %s", id)
	}
}
