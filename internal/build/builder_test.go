package build

import (
	"context"
	"testing"
)

func TestBuilder(t *testing.T) {
	t.Run("builds a PDF file from a Markdown file", func(t *testing.T) {
		ctx := context.Background()
		builder := NewTestBuilder(t, ctx)

		result, err := builder.Build(ctx, &BuildParams{})
		if err != nil {
			t.Fatalf("didn't want %q", err)
		}

		want := "..."
		got := "..."
		_ = result

		if got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func NewTestBuilder(t *testing.T, ctx context.Context) *Builder {
	panic("unimplemented")
}
