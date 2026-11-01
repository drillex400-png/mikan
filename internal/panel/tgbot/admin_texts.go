package tgbot

import "mikan/internal/textutil"

// The admin's own words, apart from the subscribers' (texts.go): nothing here is in the
// panel's setup, and the subscribers' screens never read it.
type admWords struct {
	cmdAdmin, title, notAdmin, unavailable, failed, notFound string
	overview, users, nodes, payments, back, search, refresh  string
	statUsers, statActive, statExpiring, statExpired         string
	statDisabled, statNodes, statPaid                        string
	searchAsk, searchNone, page, prev, next                  string
	card, state, until, noTerm, traffic, devices, tg, noTg   string
	extend, block, unblock, confirm, cancel, done            string
	daysAsk, daysBad, askExtend, askBlock, askUnblock        string
	nodeOn, nodeOff, noNodes, noPayments                     string
	broadcast, bcAsk, bcBad, bcPreview, bcAsk2, bcSend       string
	bcSent, bcBusy, bcOff, bcNone, bcGone                    string
}

var admRU = admWords{
	cmdAdmin: "Администрирование", title: "Администрирование", notAdmin: "Эта команда недоступна.",
	unavailable: "Панель сейчас недоступна. Попробуйте позже.", failed: "Не получилось. Попробуйте ещё раз.",
	notFound: "Подписка не найдена.",
	overview: "📊 Сводка", users: "👥 Клиенты", nodes: "🖥 Серверы", payments: "💳 Платежи",
	back: "‹ Назад", search: "🔎 Поиск", refresh: "🔄 Обновить",
	statUsers: "Всего подписок", statActive: "Активных", statExpiring: "Скоро закончатся",
	statExpired: "Закончились", statDisabled: "Заблокировано", statNodes: "Серверов",
	statPaid:   "Оплат за месяц",
	searchAsk:  "Напишите часть имени или контакта клиента.",
	searchNone: "Никого не нашли.", page: "Страница %d", prev: "‹", next: "›",
	card: "Подписка", state: "Состояние", until: "До", noTerm: "без срока",
	traffic: "Трафик", devices: "Устройства", tg: "Telegram", noTg: "не привязан",
	extend: "➕ Продлить", block: "⛔ Заблокировать", unblock: "✅ Разблокировать",
	confirm: "Да, подтверждаю", cancel: "Отмена", done: "Готово.",
	daysAsk: "На сколько дней продлить?", daysBad: "Нужно число дней от 1 до %d.",
	askExtend:  "Продлить подписку «%s» на %d дн.?",
	askBlock:   "Заблокировать подписку «%s»?",
	askUnblock: "Разблокировать подписку «%s»?",
	nodeOn:     "работает", nodeOff: "выключен", noNodes: "Серверов нет.", noPayments: "Платежей нет.",
	broadcast: "📣 Рассылка", bcAsk: "Пришлите текст рассылки. Markdown работает, как в панели. До %d символов.",
	bcBad: "Текст пустой или длиннее %d символов. Пришлите другой.", bcPreview: "Так увидят клиенты:",
	bcAsk2: "Получателей: %d. Отправить? Остановить рассылку после отправки нельзя.", bcSend: "Да, отправить",
	bcSent: "Рассылка поставлена в очередь. Получателей: %d. Ход виден в панели.", bcBusy: "Предыдущая рассылка ещё отправляется. Подождите.",
	bcOff: "Бот выключен, отправить некому.", bcNone: "Получателей нет: подписок с привязанным Telegram пока нет.",
	bcGone: "Текст рассылки потерялся (бот перезапускался). Пришлите его заново.",
}

var admEN = admWords{
	cmdAdmin: "Administration", title: "Administration", notAdmin: "This command is not available.",
	unavailable: "The panel is unavailable right now. Try again later.", failed: "That did not work. Try again.",
	notFound: "Subscription not found.",
	overview: "📊 Overview", users: "👥 Clients", nodes: "🖥 Servers", payments: "💳 Payments",
	back: "‹ Back", search: "🔎 Search", refresh: "🔄 Refresh",
	statUsers: "Subscriptions", statActive: "Active", statExpiring: "Expiring soon",
	statExpired: "Expired", statDisabled: "Blocked", statNodes: "Servers",
	statPaid:   "Payments this month",
	searchAsk:  "Type part of the client's name or contact.",
	searchNone: "Nobody found.", page: "Page %d", prev: "‹", next: "›",
	card: "Subscription", state: "State", until: "Until", noTerm: "no term",
	traffic: "Traffic", devices: "Devices", tg: "Telegram", noTg: "not linked",
	extend: "➕ Extend", block: "⛔ Block", unblock: "✅ Unblock",
	confirm: "Yes, confirm", cancel: "Cancel", done: "Done.",
	daysAsk: "Extend by how many days?", daysBad: "Enter a number of days from 1 to %d.",
	askExtend:  "Extend subscription “%s” by %d days?",
	askBlock:   "Block subscription “%s”?",
	askUnblock: "Unblock subscription “%s”?",
	nodeOn:     "up", nodeOff: "off", noNodes: "No servers.", noPayments: "No payments.",
	broadcast: "📣 Broadcast", bcAsk: "Send the broadcast text. Markdown works as in the panel. Up to %d characters.",
	bcBad: "The text is empty or longer than %d characters. Send another.", bcPreview: "This is what clients will see:",
	bcAsk2: "Recipients: %d. Send it? A broadcast cannot be stopped once sent.", bcSend: "Yes, send",
	bcSent: "The broadcast is queued. Recipients: %d. The panel shows its progress.", bcBusy: "The previous broadcast is still being sent. Wait.",
	bcOff: "The bot is off, there is no one to send it.", bcNone: "No recipients: no subscription has a linked Telegram yet.",
	bcGone: "The broadcast text was lost (the bot restarted). Send it again.",
}

func admWordsFor(lang string) *admWords { return textutil.ByLang(lang, &admRU, &admEN) }
