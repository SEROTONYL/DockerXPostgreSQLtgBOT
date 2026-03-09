package economy

func (f *fakeEconomyTG) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeEconomyTG) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}
