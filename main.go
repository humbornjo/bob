package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/humbornjo/bob/config"
	larksvc "github.com/humbornjo/bob/service/lark"
	registrysvc "github.com/humbornjo/bob/service/registry"
	"github.com/humbornjo/mizu"
	"github.com/humbornjo/mizu/mizudi"
	"github.com/humbornjo/mizu/mizulog"
	"github.com/humbornjo/mizu/mizumw/recovermw"
	"github.com/humbornjo/mizu/mizuoai"
	"github.com/humbornjo/mizu/mizuotel"
)

func main() {
	var configPaths []string
	flag.Func(
		"c", "config file path (can be specified multiple times)",
		func(s string) error {
			configPaths = append(configPaths, s)
			return nil
		},
	)
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	global := config.Initialize(true, configPaths...)

	srv := mizudi.MustRetrieve[*mizu.Server]()

	// OpenAPI ---------------------------------------------------------
	if err := mizuoai.Initialize(srv,
		config.SERVICE_NAME,
		mizuoai.WithOaiDocumentation()); err != nil {
		panic(err)
	}

	// Opentelemetry ---------------------------------------------------
	if err := mizuotel.Initialize(); err != nil {
		panic(err)
	}

	// Logging ---------------------------------------------------------
	mizulog.Initialize(
		tint.NewHandler(
			os.Stdout,
			&tint.Options{AddSource: true, TimeFormat: time.Kitchen},
		),
		mizulog.WithLogLevel(global.Level),
	)

	// Middleware ------------------------------------------------------
	srv.Use(recovermw.New())
	srv.Use(otelhttp.NewMiddleware(config.SERVICE_NAME))

	// Initialize services ---------------------------------------------
	larksvc.Initialize(ctx, global)
	registrysvc.Initialize(ctx, global)

	errChan := make(chan error, 1)

	go func() {
		defer cancel()
		defer close(errChan)
		if err := srv.ServeContext(ctx, ":"+global.Port); err != nil {
			errChan <- err
		}
	}()

	<-ctx.Done()

	if err := <-errChan; err != nil {
		slog.ErrorContext(ctx, config.SERVICE_NAME+" exit unexpectedly", "error", err)
	}
}
