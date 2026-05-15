package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"reflect"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/openapi"
	"github.com/humbornjo/mizu"
	"github.com/humbornjo/mizu/mizudi"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

const SERVICE_NAME = "bob"

var _DEFAULT_CONFIG = Config{
	Env:   "dev",
	Port:  "80",
	Level: "DEBUG",
}

//go:embed config.cue
var _RAW_SCHEMA string

func Initialize(verbose bool, paths ...string) *Config {
	mizudi.DEFAULT_UNMARSHAL_TAG = "json"
	if err := mizudi.Initialize("config", paths...); err != nil {
		panic(err)
	}

	if verbose {
		if err := mizudi.RevealConfig(os.Stdout); err != nil {
			if !errors.Is(err, mizudi.ErrNotInitialized) {
				panic(err)
			}
		}
	}

	global := mizudi.Enchant(&_DEFAULT_CONFIG)
	mizudi.Register(func() (*Config, error) { return global, nil })

	schema, err := NewSchema(_RAW_SCHEMA)
	if err != nil {
		panic(err)
	}
	SchemaMustValidate(schema, global)

	// Server ----------------------------------------------------------
	srv := mizu.NewServer(
		SERVICE_NAME,
		mizu.WithRevealRoutes(),
		mizu.WithProfilingHandlers(),
		mizu.WithServerProtocols(mizu.PROTOCOLS_HTTP2_UNENCRYPTED),
	)
	mizudi.Register(func() (*mizu.Server, error) { return srv, nil })

	{
		db, err := sqlx.Connect("postgres", global.Postgres.Dsn)
		if err != nil {
			panic(err)
		}
		mizudi.Register(func() (*sqlx.DB, error) { return db, nil })
	}

	// Initialize Volcengine TOS storage client
	{
		toscfg := global.Volcengine.Tos
		toscli, err := tos.NewClientV2(
			toscfg.Endpoint,
			tos.WithRegion(toscfg.Region),
			tos.WithCredentials(tos.NewStaticCredentials(toscfg.AccessKey, toscfg.SecretKey)),
		)
		if err != nil {
			panic(err)
		}
		mizudi.Register(func() (*tos.ClientV2, error) { return toscli, nil })
	}

	return global
}

type schema struct {
	cutex   *cue.Context
	inner   cue.Value
	openapi map[string]map[string]any
}

func NewSchema(rawSchema string) (*schema, error) {
	cuetex := cuecontext.New()
	inner := cuetex.CompileString(rawSchema)

	f, err := openapi.Generate(inner, &openapi.Config{})
	if err != nil {
		return nil, err
	}
	topValue := cuetex.BuildFile(f)
	if err := topValue.Err(); err != nil {
		return nil, err
	}
	return &schema{cutex: cuetex, inner: inner}, nil
}

func SchemaValidate[T any](schema *schema, x T) error {
	typ := reflect.TypeOf(x)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	path := "#" + typ.Name()

	unified := schema.inner.
		LookupPath(cue.ParsePath(path)).Unify(schema.cutex.Encode(x))
	if err := unified.Validate(cue.All(), cue.Definitions(true), cue.Schema()); err != nil {
		return fmt.Errorf("❌ %s", err.Error())
	}
	return nil
}

func SchemaMustValidate[T any](schema *schema, x T) {
	if err := SchemaValidate(schema, x); err != nil {
		panic(err)
	}
}

func SchemaExtractOpenAPI[T any](schema *schema, x T) (map[string]any, error) {
	if schema.openapi == nil {
		f, err := openapi.Generate(schema.inner, &openapi.Config{})
		if err != nil {
			return nil, err
		}
		topValue := schema.cutex.BuildFile(f)
		if err := topValue.Err(); err != nil {
			return nil, err
		}

		document := struct {
			Components struct {
				Schemas map[string]map[string]any `json:"schemas"`
			} `json:"components"`
		}{}
		if err := topValue.Decode(&document); err != nil {
			return nil, err
		}
		schema.openapi = document.Components.Schemas
	}

	typ := reflect.TypeOf(x)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	component := typ.Name()

	params, ok := schema.openapi[component]
	if !ok {
		return nil, fmt.Errorf("missing schema: %s", component)
	}
	if _, ok := params["properties"]; !ok {
		params["properties"] = map[string]any{}
	}

	return params, nil
}

func SchemaMustExtractOpenAPI[T any](schema *schema, x T) map[string]any {
	out, err := SchemaExtractOpenAPI(schema, x)
	if err != nil {
		panic(err)
	}
	return out
}
