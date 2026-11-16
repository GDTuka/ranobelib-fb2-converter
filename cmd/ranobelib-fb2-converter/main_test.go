package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunRequiresConfigFlag(t *testing.T) {
	err := run(context.Background(), nil, &strings.Builder{}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "-config is required") {
		t.Fatalf("run() error = %v", err)
	}
}
