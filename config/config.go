package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"reflect"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/humbornjo/mizu/mizudi"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

const SERVICE_NAME = "bob"

var _DEFAULT_CONFIG = Config{
	Env:   "dev",
	Port:  "80",
	Level: "DEBUG",
}

//go:embed config.cue
var _SCHEMA string

func Initialize(paths ...string) *Config {
	mizudi.DEFAULT_UNMARSHAL_TAG = "json"
	if err := mizudi.Initialize("config", paths...); err != nil {
		panic(err)
	}

	if err := mizudi.RevealConfig(os.Stdout); err != nil {
		if !errors.Is(err, mizudi.ErrNotInitialized) {
			panic(err)
		}
	}

	c := mizudi.Enchant(&_DEFAULT_CONFIG)
	if err := Validate(_SCHEMA, c); err != nil {
		panic(err)
	}

	mizudi.Register(func() (*Config, error) { return c, nil })

	// Initialize Volcengine TOS storage client
	{
		toscfg := c.Volcengine.Tos
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

	return c
}

func Validate[T any](rawSchema string, x T) error {
	typ := reflect.TypeOf(x)
	name := typ.Elem().Name()
	path := "#" + name

	cuetex := cuecontext.New()
	schema := cuetex.CompileString(rawSchema)

	unified := schema.LookupPath(cue.ParsePath(path)).Unify(cuetex.Encode(x))
	if err := unified.Validate(cue.All(), cue.Definitions(true), cue.Schema()); err != nil {
		return fmt.Errorf("❌ %s", err.Error())
	}
	return nil
}
