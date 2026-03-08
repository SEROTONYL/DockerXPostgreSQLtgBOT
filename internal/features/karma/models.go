package karma

import "time"

const (
	ThanksReward             int64 = 20
	DefaultThanksDailyLimit        = 3
	ThanksReciprocalCooldown       = 5 * time.Minute
	thanksRewardTxType             = "thanks_reward"
)

type ThanksStats struct {
	SentCount      int
	ReceivedCount  int
	ReceivedReward int64
}
