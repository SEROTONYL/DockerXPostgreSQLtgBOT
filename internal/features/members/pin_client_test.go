package members

func (f *fakeMembersTG) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeMembersTG) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}

func (f *fakeMembersTGWithOptions) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeMembersTGWithOptions) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}

func (f *fakeTelegramClient) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeTelegramClient) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}
