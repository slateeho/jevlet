package signals

import "testing"

func TestRenderQuery(t *testing.T) {
	got := RenderQuery(`rate(x{namespace="{{namespace}}",pod="{{pod}}"}[5m])`, "app", "backend-123")
	want := `rate(x{namespace="app",pod="backend-123"}[5m])`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
