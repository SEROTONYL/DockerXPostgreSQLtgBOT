package filters

func (f *fakeTG) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeTG) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}

func (f *fakeTGMember) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeTGMember) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}
