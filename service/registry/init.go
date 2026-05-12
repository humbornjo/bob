package registrysvc

import (
	"context"

	"github.com/humbornjo/bob/config"
	"github.com/humbornjo/mizu"
	"github.com/humbornjo/mizu/mizudi"
	"github.com/humbornjo/mizu/mizuoai"
)

func Initialize(ctx context.Context, global *config.Config) {
	srv := mizudi.MustRetrieve[*mizu.Server]()
	svc := Service{}

	group := srv.Group("/registry")

	mizuoai.Post(
		group, "/skill:view", svc.HandleViewSkill,
		mizuoai.WithOperationTags("registry", "skill"),
		mizuoai.WithOperationDescription(`List all available skills (progressive disclosure tier 1 - minimal metadata).

    Returns only name + description to minimize token usage.`),
	)
	mizuoai.Post(
		group, "/skill:list", svc.HandleListSkills,
		mizuoai.WithOperationTags("registry", "skill"),
		mizuoai.WithOperationDescription(`List all available skills (progressive disclosure tier 1 - minimal metadata).

    Returns only name + description to minimize token usage.`),
	)
}
