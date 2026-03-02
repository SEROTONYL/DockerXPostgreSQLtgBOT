# uiwizard

Лёгкий engine для single-message wizard flow.

## Как использовать
1. Создай `WizardState` при старте флоу (`ChatID`, `MessageID`, `StartedAt`, `Step`).
2. На каждом шаге делай `Transition(...)` и рендери через `Render(...)`.
3. Для callback проверяй `EnsureStep(...)`/`Require(...)`.
4. Для текстового ввода используй `AwaitTextFor` + `IsAwaitingText`/`ConsumeText`.
5. При выходе/ошибке вызывай `Reset(...)`.
