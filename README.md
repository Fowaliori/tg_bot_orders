# Телеграм бот для принятия заявок
Пользователь в чате отправляет данные боту, у админа появляется заказ, который он может удалить после выполнения. Используется tgbotapi

## Технологии
- Go
- TgBotAPI

## Работа с проектом
Установить необходимые библиотеки
```bash
go install github.com/go-telegram-bot-api/telegram-bot-api/v5
go install github.com/joho/godotenv
```
В .env указать token телеграм бота(не показывать никому!)

Запуск бота
```bash
go run main.go
```

