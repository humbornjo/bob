package larksvc

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"

	"github.com/chyroc/lark"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"

	"github.com/humbornjo/bob/config"
	"github.com/humbornjo/bob/package/storage"
	"github.com/humbornjo/mizu"
	"github.com/humbornjo/mizu/mizudi"
)

//go:embed config.cue
var _SCHEMA string

func Initialize(global *config.Config) {
	local := mizudi.Enchant(&Config{})
	config.Validate(_SCHEMA, "#Config", local)

	srv := mizudi.MustRetrieve[*mizu.Server]()
	group := srv.Group("/lark")

	var svc = &Service{model: local.Model.Name}

	// Initialize oos storage
	{
		toscli := mizudi.MustRetrieve[*tos.ClientV2]()
		toscfg := global.Volcengine.Tos
		svc.Storage = storage.FromVolc(toscli, toscfg.Bucket, local.Storage.Folder)
	}

	// Initialize mcp client
	for _, mcpsrv := range local.Model.McpServers {
		slog.Debug("initialize mcp client", "endpoint", mcpsrv.Endpoint)
		transport, err := transport.NewSSE(mcpsrv.Endpoint, transport.WithHeaders(mcpsrv.Headers))
		if err != nil {
			panic(err)
		}
		mcpcli := client.NewClient(transport)
		if _, err := mcpcli.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
			panic(err)
		}
		svc.mcpclis = append(svc.mcpclis, mcpcli)
	}

	// Initialize lark client
	{
		appId := os.Getenv("LARK_APP_ID")
		appSecret := os.Getenv("LARK_APP_SECRET")
		svc.larkcli = lark.New(lark.WithAppCredential(appId, appSecret))

		eventHandler := dispatcher.NewEventDispatcher("-", "-").
			// OnP1MessageReadV1(svc.HandleP1MessageReadV1).
			// OnP1MessageReceiveV1(svc.HandleP1MessageReceiveV1).
			OnP2MessageReadV1(svc.HandleP2MessageReadV1).
			OnP2MessageReceiveV1(svc.HandleP2MessageReceiveV1)
		srvws := larkws.NewClient(
			appId, appSecret,
			larkws.WithEventHandler(eventHandler),
			larkws.WithLogLevel(larkcore.LogLevelDebug),
			larkws.WithLogger(logger{slog.Default()}),
		)
		mizudi.Register(func() (*larkws.Client, error) { return srvws, nil })
	}

	group.HandleFunc("/chat", svc.HandleSendMessage)
}

var _ larkcore.Logger = (*logger)(nil)

type logger struct {
	*slog.Logger
}

func (l logger) Debug(ctx context.Context, args ...any) {
	if len(args) == 0 {
		args = append(args, "-")
	}
	l.DebugContext(ctx, fmt.Sprintf("%v", args[0]), args[1:]...)
}

func (l logger) Info(ctx context.Context, args ...any) {
	if len(args) == 0 {
		args = append(args, "-")
	}
	l.InfoContext(ctx, fmt.Sprintf("%v", args[0]), args[1:]...)
}

func (l logger) Warn(ctx context.Context, args ...any) {
	if len(args) == 0 {
		args = append(args, "-")
	}
	l.WarnContext(ctx, fmt.Sprintf("%v", args[0]), args[1:]...)
}

func (l logger) Error(ctx context.Context, args ...any) {
	if len(args) == 0 {
		args = append(args, "-")
	}
	l.ErrorContext(ctx, fmt.Sprintf("%v", args[0]), args[1:]...)
}
