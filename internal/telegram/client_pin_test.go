package telegram

func (f *fakeClient) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeClient) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}

func (f *callbackPreferenceClient) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *callbackPreferenceClient) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}
