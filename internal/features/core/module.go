package core

import (
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type Deps struct {
	Ops *telegram.Ops
}

func Build(deps Deps) (feature.Feature, error) {
	return NewFeature(deps.Ops), nil
}
