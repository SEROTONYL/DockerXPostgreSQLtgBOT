package bot

import (
	"context"

	models "github.com/mymmrac/telego"
)

func (f *fakeTGStatus) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeTGStatus) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}

func (a *adminHandlerRecorder) HandleRiddleMessage(ctx context.Context, message *models.Message) bool {
	return false
}
