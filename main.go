package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/lmittmann/tint"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/humbornjo/bob/config"
	larksvc "github.com/humbornjo/bob/service/lark"
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

	global := config.Initialize(configPaths...)

	// Server ----------------------------------------------------------
	srv := mizu.NewServer(
		config.ServiceName,
		mizu.WithRevealRoutes(),
		mizu.WithProfilingHandlers(),
		mizu.WithServerProtocols(mizu.PROTOCOLS_HTTP2_UNENCRYPTED),
	)
	mizudi.Register(func() (*mizu.Server, error) { return srv, nil })

	// OpenAPI ---------------------------------------------------------
	if err := mizuoai.Initialize(srv,
		config.ServiceName,
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
	srv.Use(otelhttp.NewMiddleware(config.ServiceName))

	// Initialize services ---------------------------------------------
	larksvc.Initialize(global)

	errChan := make(chan error, 1)

	go func() {
		srvws := mizudi.MustRetrieve[*larkws.Client]()
		if err := srvws.Start(ctx); err != nil {
			slog.ErrorContext(ctx, "lark ws exit unexpectedly", "error", err)
			errChan <- err
		}
	}()

	go func() {
		defer cancel()
		defer close(errChan)
		if err := srv.ServeContext(ctx, ":"+global.Port); err != nil {
			errChan <- err
		}
	}()

	<-ctx.Done()

	if err := <-errChan; err != nil {
		slog.ErrorContext(ctx, config.ServiceName+" exit unexpectedly", "error", err)
	}
}
