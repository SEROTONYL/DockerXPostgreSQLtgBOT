package members

import "serotonyl.ru/telegram-bot/internal/feature"

type Deps struct{}

func Build(Deps) (feature.Feature, error) {
	return NewFeature(), nil
}
