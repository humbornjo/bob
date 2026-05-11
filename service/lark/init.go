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
	anyllm "github.com/mozilla-ai/any-llm-go"
	anyllmconfig "github.com/mozilla-ai/any-llm-go/config"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"

	"github.com/humbornjo/bob/config"
	"github.com/humbornjo/bob/package/llm"
	llmmcp "github.com/humbornjo/bob/package/llm/mcp"
	"github.com/humbornjo/bob/package/storage"
	"github.com/humbornjo/mizu"
	"github.com/humbornjo/mizu/mizudi"
	"github.com/humbornjo/mizu/mizuoai"
)

//go:embed config.cue
var _SCHEMA string

func Initialize(global *config.Config) {
	local := mizudi.Enchant(&Config{})
	if err := config.Validate(_SCHEMA, local); err != nil {
		panic(err)
	}

	var svc = &Service{model: local.Model.Name}
	srv := mizudi.MustRetrieve[*mizu.Server]()

	group := srv.Group("/lark")
	mizuoai.Post(group, "/chat", svc.HandleSendMessage)

	// Initialize oos storage
	{
		toscli := mizudi.MustRetrieve[*tos.ClientV2]()
		toscfg := global.Volcengine.Tos
		svc.Storage = storage.FromVolcTOS(toscli, toscfg.Bucket, local.Storage.Folder)
	}

	// Initialize mcp client
	for _, mcpsrv := range local.Model.McpServers {
		client, err := llmmcp.NewClient(
			mcpsrv.Transport,
			llmmcp.WithEnabledTools(mcpsrv.EnabledTools...),
			llmmcp.WithDisabledTools(mcpsrv.DisabledTools...),
			llmmcp.WithToolExtensions(mcpsrv.ToolExtensions...),
		)
		if err != nil {
			panic(err)
		}
		svc.mcpclis = append(svc.mcpclis, client)
	}

	// Initialize lark client
	{
		appID := os.Getenv("LARK_APP_ID")
		appSecret := os.Getenv("LARK_APP_SECRET")
		svc.larkcli = lark.New(lark.WithAppCredential(appID, appSecret))

		// Initialize app specific const values
		{
			var tenantToken string
			{
				resp, _, err := svc.larkcli.Auth.GetTenantAccessToken(context.Background())
				if err != nil {
					panic(err)
				}
				tenantToken = resp.Token
			}
			resp, _, err := svc.larkcli.Bot.GetBotInfo(
				context.Background(), &lark.GetBotInfoReq{},
				lark.WithRequestHeaders(map[string]string{"Authorization": "Bearer " + tenantToken}),
			)
			if err != nil {
				panic(err)
			}
			svc.appname = resp.AppName
			svc.appid = appID
			svc.openid = resp.OpenID
		}
		slog.Debug("initialize lark client", "app_name", svc.appname, "app_id", svc.appid, "open_id", svc.openid)

		eventHandler := dispatcher.NewEventDispatcher("-", "-").
			// OnP1MessageReadV1(svc.HandleP1MessageReadV1).
			// OnP1MessageReceiveV1(svc.HandleP1MessageReceiveV1).
			OnP2MessageReadV1(svc.HandleP2MessageReadV1).
			OnP2MessageReceiveV1(svc.HandleP2MessageReceiveV1)

		srvws := larkws.NewClient(
			appID, appSecret,
			larkws.WithEventHandler(eventHandler),
			larkws.WithLogger(logger{slog.Default()}),
		)
		mizudi.Register(func() (*larkws.Client, error) { return srvws, nil })
	}

	// Initialize model provider
	{
		var err error
		var provider anyllm.Provider
		switch local.Model.Provider {
		case llm.PROVIDER_OPENAI:
			apiKey, baseUrl := os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_BASE_URL")
			provider, err = openai.New(anyllmconfig.WithAPIKey(apiKey), anyllmconfig.WithBaseURL(baseUrl))
		default:
			panic("unknown LLM provider: " + local.Model.Provider)
		}
		if err != nil {
			panic(err)
		}
		svc.provider = provider
	}
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
