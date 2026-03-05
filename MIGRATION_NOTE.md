# Telegram Client Migration Note

## Scope

This project was migrated from:

- `github.com/go-telegram/bot`
- `github.com/go-telegram/bot/models`

to:

- `github.com/mymmrac/telego`

Business logic, feature wiring, command routing, and DB behavior were intentionally preserved.

## Key mapping decisions

- `models.Update` -> `telego.Update`
- `models.Message` -> `telego.Message`
- `models.CallbackQuery` -> `telego.CallbackQuery`
- `models.InlineKeyboardMarkup` -> `telego.InlineKeyboardMarkup`
- `models.InlineKeyboardButton` -> `telego.InlineKeyboardButton`

### Telegram API calls

- `SendMessage` now uses `telego.SendMessageParams` with `ChatID: telego.ChatID{ID: chatID}`.
- `EditMessageText` now uses `telego.EditMessageTextParams` with `ChatID: telego.ChatID{ID: chatID}`.
- `EditMessageReplyMarkup` now uses `telego.EditMessageReplyMarkupParams`.
- `DeleteMessage` now uses `telego.DeleteMessageParams`.
- `GetChatMember` now uses `telego.GetChatMemberParams` and returns `telego.ChatMember` interface.
- `AnswerCallbackQuery` now maps directly to `telego.AnswerCallbackQueryParams`.

### Runtime/update handling

- Existing internal abstraction (`internal/telegram.Client` + `internal/telegram.Ops`) was kept.
- Update registration semantics were preserved via internal handler registry.
- Bot start now consumes updates through `Bot.UpdatesViaLongPolling(...)` and dispatches to registered handlers.

## Type compatibility notes

- Message ID field changed from `Message.ID` to `Message.MessageID`.
- Callback query message is `MaybeInaccessibleMessage`; code now accesses `q.Message.Message()` safely.
- Chat member is an interface in telego (`telego.ChatMember`):
  - status checks use `MemberStatus()` (string values like `creator`, `administrator`, `member`, `restricted`, `left`, `kicked`)
  - user extraction uses `MemberUser()`

## What did NOT change

- Command routes and handler flow.
- Feature boundaries and architecture.
- Database schema and DB operations.
- Admin/karma/streak/economy/casino feature behavior.
