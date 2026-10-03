package slogpretty

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestHandleError(t *testing.T) {
	for _, withAttrs := range []bool{false, true} {
		t.Run(fmt.Sprint(withAttrs), func(t *testing.T) {
			var output bytes.Buffer
			var handler slog.Handler = (PrettyHandlerOptions{}).NewPrettyHandler(&output)
			attr := slog.Any("error", fmt.Errorf("fetch goods: %w", errors.New("connection refused")))
			record := slog.NewRecord(time.Date(2026, 10, 3, 21, 17, 5, 0, time.UTC), slog.LevelError, "scheduled job failed", 0)
			if withAttrs {
				handler = handler.WithAttrs([]slog.Attr{attr})
			} else {
				record.AddAttrs(attr)
			}
			if err := handler.Handle(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{`"error": "fetch goods: connection refused"`, "[21:17:05.000]"} {
				if !strings.Contains(output.String(), want) {
					t.Fatalf("output %q does not contain %q", output.String(), want)
				}
			}
		})
	}
}
