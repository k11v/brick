package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun(t *testing.T) {
	t.Run("runs", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping in short mode")
		}

		tempDir := t.TempDir()

		inputDir := filepath.Join(tempDir, "input")
		if err := os.MkdirAll(inputDir, 0o777); err != nil {
			t.Fatalf("got %q err", err)
		}
		mdFile := filepath.Join(inputDir, "main.md")
		err := os.WriteFile(
			mdFile,
			[]byte(`# The Hobbit, or There and Back Again

## Text

Once upon a time, in the depths of the quiet ocean, there lived a small fish named Flora. Flora was special - she had bright colors and long fins that allowed her to swim quickly. She was curious and always eager to explore new places in the ocean.

One day, during her adventures, Flora noticed a large school of her fellow fish migrating north. She decided to join them and explore new places. During the journey, Flora met many different species of fish. She learned that many fish cooperate with each other to find food and protect themselves from predators.

Soon, Flora discovered a huge coral reef community where hundreds of colorful fish lived. They lived in harmony and cared for each other. Flora stayed there and learned a lot from her new friends. She realized that unity and cooperation were key to survival in the ocean.

Over the years, Flora grew older and wiser. She became one of the elders of the coral reef and helped young fish in their journey through the ocean. Her story became a legend among the fish and an inspiration to many. In her old age, Flora felt proud of all she had achieved and thanked the ocean for the amazing adventures and friendships she found along the way.

### Formatting

Text with *italics*.

Text with **bold**.

Text with ***bold italics***.

Text with `+"`code`"+`.

Text with $E = mc^2$.

Text with "'quotes' inside quotes".

Text with 🤔.

Text with кириллица.
`),
			0o666,
		)
		if err != nil {
			t.Fatalf("got %q err", err)
		}

		outputDir := filepath.Join(tempDir, "output")
		result, err := Build(&BuildParams{InputDir: inputDir, OutputDir: outputDir})
		if err != nil {
			t.Fatalf("got %q err", err)
		}
		if got, want := result.ExitCode, 0; got != want {
			t.Errorf("got %d ExitCode, want %d", got, want)
		}
		if got := result.PDFFile; got == "" {
			t.Error("got empty PDFFile")
		}
		if got := result.LogFile; got == "" {
			t.Error("got empty LogFile")
		}
	})
}
