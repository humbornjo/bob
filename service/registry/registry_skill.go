package registrysvc

import "github.com/humbornjo/mizu/mizuoai"

type Service struct {
}

// HandleListSkills --------------------------------------------------
type HandleListSkillsInput struct {
	Body struct {
		Scope string `json:"scope"`
	} `mizu:"body"`
}

type HandleListSkillsOutput = string

func (s *Service) HandleListSkills(tx mizuoai.Tx[HandleListSkillsOutput], rx mizuoai.Rx[HandleListSkillsInput]) {
}

// HandleViewSkill ---------------------------------------------------
type HandleViewSkillInput struct {
	Body struct {
		Name string `json:"name"`
	} `mizu:"body"`
}

type HandleViewSkillOutput = string

func (s *Service) HandleViewSkill(tx mizuoai.Tx[HandleViewSkillOutput], rx mizuoai.Rx[HandleViewSkillInput]) {
}
