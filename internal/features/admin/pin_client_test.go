package admin

func (f *fakeTG) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	f.calls = append(f.calls, tgCall{kind: "pin", chatID: chatID, messageID: messageID})
	return f.pinErr
}

func (f *fakeTG) UnpinChatMessage(chatID int64, messageID int) error {
	f.calls = append(f.calls, tgCall{kind: "unpin", chatID: chatID, messageID: messageID})
	return f.unpinErr
}
