package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

// Структура для хранения данных пользователя
type UserData struct {
	UserID       int64  `json:"user_id"`
	GameMode     string `json:"game_mode"`
	Login        string `json:"login"`
	Password     string `json:"password"`
	TwitchNick   string `json:"twitch_nick"`
	TelegramUser string `json:"telegram_user"`
	CreatedAt    string `json:"created_at"`
}

// Структура для хранения состояния пользователя
type UserState struct {
	WaitingFor   string
	TempUserData UserData
	DeleteIndex  int
}

var (
	userStates = make(map[int64]*UserState)
	adminUsers = map[int64]bool{
		295221178: true,
	}
	moscowLocation *time.Location
)

func init() {
	var err error
	moscowLocation, err = time.LoadLocation("Europe/Moscow")
	if err != nil {
		moscowLocation = time.FixedZone("MSK", 3*60*60)
		log.Printf("Используем фиксированную зону для Москвы: UTC+3")
	}
}

func getMoscowTime() string {
	now := time.Now().In(moscowLocation)
	return now.Format("02.01.2006 15:04:05 (MST)")
}

func getMoscowTimeForDisplay() string {
	now := time.Now().In(moscowLocation)
	return now.Format("02.01.2006 в 15:04")
}

// Клавиатура выбора режима игры
var gameModeKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("MOC"),
		tgbotapi.NewKeyboardButton("PF"),
		tgbotapi.NewKeyboardButton("APOC"),
	),
)

// Клавиатура да/нет
var yesNoKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("Да"),
		tgbotapi.NewKeyboardButton("Нет"),
	),
)

// Клавиатура для админа
var adminKeyboard = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Удалить все", "delete_all"),
		tgbotapi.NewInlineKeyboardButtonData("📋 Список", "list_orders"),
	),
)

// Клавиатура подтверждения удаления
func getConfirmationKeyboard(action string, index int) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", fmt.Sprintf("confirm_%s_%d", action, index)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Нет, отмена", "cancel_delete"),
		),
	)
}

// Функция для проверки формата username
func isValidTelegramUsername(username string) bool {
	if username == "" {
		return false
	}
	if !strings.HasPrefix(username, "@") {
		return false
	}

	cleanUsername := username[1:]
	if len(cleanUsername) < 5 || len(cleanUsername) > 32 {
		return false
	}

	for _, char := range cleanUsername {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_') {
			return false
		}
	}

	return true
}

// Функция для сохранения данных в JSON - ИСПРАВЛЕНА
func saveUserData(userData UserData) error {
	// Добавляем время создания
	userData.CreatedAt = getMoscowTime()

	file, err := os.OpenFile("users.json", os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	var users []UserData
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&users); err != nil && err.Error() != "EOF" {
		users = []UserData{}
	}

	// ИСПРАВЛЕНИЕ: Ищем и обновляем существующий заказ этого пользователя
	found := false
	for i, user := range users {
		if user.UserID == userData.UserID {
			users[i] = userData // Обновляем существующий заказ
			found = true
			break
		}
	}

	// Если заказ не найден, добавляем новый (а не заменяем все)
	if !found {
		users = append(users, userData)
	}

	file.Seek(0, 0)
	file.Truncate(0)
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(users)
}

// Функция для чтения всех заказов
func getAllOrders() ([]UserData, error) {
	file, err := os.Open("users.json")
	if err != nil {
		if os.IsNotExist(err) {
			return []UserData{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var users []UserData
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&users); err != nil && err.Error() != "EOF" {
		return nil, err
	}

	return users, nil
}

// Функция для удаления всех заказов
func deleteAllOrders() error {
	file, err := os.OpenFile("users.json", os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode([]UserData{})
}

// Функция для удаления конкретного заказа по индексу
func deleteOrder(index int) error {
	orders, err := getAllOrders()
	if err != nil {
		return err
	}

	if index < 0 || index >= len(orders) {
		return fmt.Errorf("неверный индекс заказа")
	}

	orders = append(orders[:index], orders[index+1:]...)

	file, err := os.OpenFile("users.json", os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(orders)
}

// Функция для проверки является ли пользователь админом
func isAdmin(userID int64) bool {
	return adminUsers[userID]
}

// Функция для форматирования списка заказов
func formatOrders(orders []UserData, withIndex bool) string {
	if len(orders) == 0 {
		return "📭 Нет активных заказов"
	}

	response := fmt.Sprintf("📋 Все заказы (%d):\n\n", len(orders))
	for i, order := range orders {
		if withIndex {
			response += fmt.Sprintf("🔢 #%d\n", i+1)
		}
		response += fmt.Sprintf("👤 User ID: %d\n", order.UserID)
		response += fmt.Sprintf("📱 Telegram: %s\n", order.TelegramUser)
		response += fmt.Sprintf("🎮 Режим: %s\n", order.GameMode)
		response += fmt.Sprintf("🔑 Логин: %s\n", order.Login)
		response += fmt.Sprintf("🔒 Пароль: %s\n", order.Password)
		response += fmt.Sprintf("📺 Twitch: %s\n", order.TwitchNick)
		response += fmt.Sprintf("🕐 Создан: %s\n", order.CreatedAt)
		response += "────────────────────\n"
	}
	return response
}

// Функция для получения статистики с временными данными
func getOrdersStats() string {
	orders, err := getAllOrders()
	if err != nil {
		return "❌ Ошибка при получении статистики: " + err.Error()
	}

	if len(orders) == 0 {
		return "📊 Статистика заказов:\n📦 Всего заказов: 0"
	}

	stats := make(map[string]int)
	var latestOrder time.Time

	for _, order := range orders {
		stats[order.GameMode]++

		if orderTime, err := time.Parse("02.01.2006 15:04:05 (MST)", order.CreatedAt); err == nil {
			if orderTime.After(latestOrder) {
				latestOrder = orderTime
			}
		}
	}

	response := "📊 Статистика заказов:\n"
	response += fmt.Sprintf("📦 Всего заказов: %d\n", len(orders))
	for mode, count := range stats {
		response += fmt.Sprintf("🎮 %s: %d\n", mode, count)
	}

	if !latestOrder.IsZero() {
		response += fmt.Sprintf("⏰ Последний заказ: %s\n", latestOrder.Format("02.01.2006 в 15:04"))
	}

	return response
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			userID := update.CallbackQuery.From.ID
			if isAdmin(userID) {
				data := update.CallbackQuery.Data
				chatID := update.CallbackQuery.Message.Chat.ID

				var responseText string

				switch {
				case data == "delete_all":
					responseText = "⚠️ Вы уверены, что хотите удалить ВСЕ заказы?\n\nЭта операция необратима!"
					msg := tgbotapi.NewMessage(chatID, responseText)
					msg.ReplyMarkup = getConfirmationKeyboard("all", -1)
					if _, err := bot.Send(msg); err != nil {
						log.Panic(err)
					}

				case data == "list_orders":
					// ИСПРАВЛЕНИЕ: Редактируем существующее сообщение вместо отправки нового
					orders, err := getAllOrders()
					if err != nil {
						responseText = "❌ Ошибка при получении заказов: " + err.Error()
					} else {
						responseText = formatOrders(orders, true)
					}

					// Редактируем сообщение с кнопками
					editMsg := tgbotapi.NewEditMessageText(chatID, update.CallbackQuery.Message.MessageID, responseText)
					if _, err := bot.Send(editMsg); err != nil {
						log.Panic(err)
					}

				case strings.HasPrefix(data, "confirm_"):
					parts := strings.Split(data, "_")
					if len(parts) >= 3 {
						action := parts[1]
						index, _ := strconv.Atoi(parts[2])

						switch action {
						case "all":
							if err := deleteAllOrders(); err != nil {
								responseText = "❌ Ошибка при удалении заказов: " + err.Error()
							} else {
								responseText = "✅ Все заказы успешно удалены!"
							}
						case "order":
							if err := deleteOrder(index); err != nil {
								responseText = "❌ Ошибка при удалении заказа: " + err.Error()
							} else {
								responseText = fmt.Sprintf("✅ Заказ #%d успешно удален!", index+1)
							}
						}
					}

				case data == "cancel_delete":
					responseText = "❌ Удаление отменено"

				default:
					responseText = "Неизвестная команда"
				}

				// Отправляем ответ только для подтверждения/отмены (не для list_orders)
				if responseText != "" && data != "list_orders" {
					msg := tgbotapi.NewMessage(chatID, responseText)
					if _, err := bot.Send(msg); err != nil {
						log.Panic(err)
					}
				}

				// Ответ на callback (убирает часики у кнопки)
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
				if _, err := bot.Request(callback); err != nil {
					log.Panic(err)
				}
			}
			continue
		}

		if update.Message == nil {
			continue
		}

		userID := update.Message.Chat.ID
		var msg tgbotapi.MessageConfig

		state, exists := userStates[userID]

		if update.Message.Text == "MOC" || update.Message.Text == "PF" || update.Message.Text == "APOC" {
			if state, exists := userStates[userID]; exists && state.WaitingFor == "game_mode" {
				state.TempUserData.GameMode = update.Message.Text
				state.WaitingFor = "artifact_change"
				msg = tgbotapi.NewMessage(userID, "Можно менять артефакты на персонажах?")
				msg.ReplyMarkup = yesNoKeyboard
			} else {
				msg = tgbotapi.NewMessage(userID, "Пожалуйста, начните процесс с помощью /neworder")
			}

			if _, err := bot.Send(msg); err != nil {
				log.Panic(err)
			}
			continue
		}

		if isAdmin(userID) {
			switch update.Message.Command() {
			case "orders":
				orders, err := getAllOrders()
				if err != nil {
					msg = tgbotapi.NewMessage(userID, "Ошибка при чтении заказов: "+err.Error())
				} else {
					msg = tgbotapi.NewMessage(userID, formatOrders(orders, true))
					msg.ReplyMarkup = adminKeyboard
				}

			case "stats":
				msg = tgbotapi.NewMessage(userID, getOrdersStats())

			case "delete":
				msg = tgbotapi.NewMessage(userID, "⚠️ Вы уверены, что хотите удалить ВСЕ заказы?\n\nЭта операция необратима!")
				msg.ReplyMarkup = getConfirmationKeyboard("all", -1)

			case "deleteorder":
				args := strings.Fields(update.Message.Text)
				if len(args) < 2 {
					msg = tgbotapi.NewMessage(userID, "Используйте: /deleteorder <номер_заказа>")
				} else {
					index, err := strconv.Atoi(args[1])
					if err != nil || index < 1 {
						msg = tgbotapi.NewMessage(userID, "Неверный номер заказа")
					} else {
						orders, err := getAllOrders()
						if err != nil {
							msg = tgbotapi.NewMessage(userID, "Ошибка: "+err.Error())
						} else if index > len(orders) {
							msg = tgbotapi.NewMessage(userID, fmt.Sprintf("Заказа #%d не существует", index))
						} else {
							if userStates[userID] == nil {
								userStates[userID] = &UserState{}
							}
							userStates[userID].DeleteIndex = index - 1

							order := orders[index-1]
							msg = tgbotapi.NewMessage(userID,
								fmt.Sprintf("⚠️ Вы уверены, что хотите удалить заказ #%d?\n\n"+
									"👤 User ID: %d\n"+
									"📱 Telegram: %s\n"+
									"🎮 Режим: %s\n"+
									"🔑 Логин: %s\n"+
									"📺 Twitch: %s\n"+
									"🕐 Создан: %s\n\n"+
									"Эта операция необратима!",
									index, order.UserID, order.TelegramUser, order.GameMode, order.Login, order.TwitchNick, order.CreatedAt))
							msg.ReplyMarkup = getConfirmationKeyboard("order", index-1)
						}
					}
				}

			case "adminhelp":
				helpText := `🛠️ Команды администратора:

/orders - Показать все заказы
/stats - Статистика заказов 
/delete - Удалить ВСЕ заказы 
/deleteorder <номер> - Удалить конкретный заказ
/adminhelp - Помощь по командам админа`
				msg = tgbotapi.NewMessage(userID, helpText)
			}

			if msg.Text != "" {
				if _, err := bot.Send(msg); err != nil {
					log.Panic(err)
				}
				continue
			}
		}

		if exists && state.WaitingFor != "" {
			switch state.WaitingFor {
			case "artifact_change":
				if update.Message.Text == "Да" || update.Message.Text == "Нет" {
					msg = tgbotapi.NewMessage(userID, "Введите ваш логин:")
					state.WaitingFor = "login"
					msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
				} else {
					msg = tgbotapi.NewMessage(userID, "Пожалуйста, выберите 'Да' или 'Нет' на клавиатуре:")
					msg.ReplyMarkup = yesNoKeyboard
				}

			case "login":
				state.TempUserData.Login = update.Message.Text
				msg = tgbotapi.NewMessage(userID, "Введите ваш пароль:")
				state.WaitingFor = "password"

			case "password":
				state.TempUserData.Password = update.Message.Text
				msg = tgbotapi.NewMessage(userID, "Введите ваш ник на Twitch:")
				state.WaitingFor = "twitch_nick"

			case "twitch_nick":
				state.TempUserData.TwitchNick = update.Message.Text
				msg = tgbotapi.NewMessage(userID, "Введите ваш username в Telegram (в формате @example):")
				state.WaitingFor = "telegram_user"

			case "telegram_user":
				if isValidTelegramUsername(update.Message.Text) {
					state.TempUserData.TelegramUser = update.Message.Text
					state.TempUserData.UserID = userID

					if err := saveUserData(state.TempUserData); err != nil {
						msg = tgbotapi.NewMessage(userID, "Ошибка при сохранении данных: "+err.Error())
					} else {
						creationTime := getMoscowTimeForDisplay()
						msg = tgbotapi.NewMessage(userID, "✅ Данные успешно сохранены!\n\n"+
							"Режим игры: "+state.TempUserData.GameMode+"\n"+
							"Логин: "+state.TempUserData.Login+"\n"+
							"Пароль: ********\n"+
							"Twitch ник: "+state.TempUserData.TwitchNick+"\n"+
							"Telegram: "+state.TempUserData.TelegramUser+"\n"+
							"🕐 Заказ создан: "+creationTime)
					}

					delete(userStates, userID)
					msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
				} else {
					msg = tgbotapi.NewMessage(userID, "❌ Неверный формат username!\n\n"+
						"Username должен начинаться с @ и содержать только буквы, цифры и underscore.\n"+
						"Пример: @example_user\n\n"+
						"Пожалуйста, введите ваш username еще раз:")
					state.WaitingFor = "telegram_user"
				}

			default:
				msg = tgbotapi.NewMessage(userID, "Неизвестное состояние. Начните заново с /neworder")
				delete(userStates, userID)
			}

		} else {
			switch update.Message.Command() {
			case "start":
				msg = tgbotapi.NewMessage(userID, "🚀 Добро пожаловать!.\n\nИспользуйте /help для просмотра доступных команд.")
				msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)

			case "help":
				helpText := `📋 Доступные команды:

/start - Начать работу с ботом
/help - Показать команды
/neworder - Создать новую запись`

				if isAdmin(userID) {
					helpText += "\n\n🛠️ Команды администратора доступны через /adminhelp"
				}

				msg = tgbotapi.NewMessage(userID, helpText)

			case "neworder":
				userStates[userID] = &UserState{
					WaitingFor:   "game_mode",
					TempUserData: UserData{},
				}
				msg = tgbotapi.NewMessage(userID, "🎮 Выберите режим игры:")
				msg.ReplyMarkup = gameModeKeyboard

			default:
				msg = tgbotapi.NewMessage(userID, "🤖 Я получил ваше сообщение: \""+update.Message.Text+"\"\n\nИспользуйте /help для просмотра команд.")
			}
		}

		if _, err := bot.Send(msg); err != nil {
			log.Panic(err)
		}
	}
}
