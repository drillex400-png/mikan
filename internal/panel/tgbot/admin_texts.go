package tgbot

import "mikan/internal/textutil"

// The admin's own words, apart from the subscribers' (texts.go): nothing here is in the
// panel's setup, and the subscribers' screens never read it.
type admWords struct {
	cmdAdmin, title, notAdmin, unavailable, failed, notFound string
	failOld, failInput                                       string
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

	// States, payments and time, in words.
	stActive, stExpiring, stExpired, stLimited, stDisabled string
	pPaid, pPending, pFailed, pExpired, pRefunded          string
	endedToday, endedAgo, endsToday, endsIn                string

	// The card and what it changes.
	cardTariff, cardNote, cardSince, noTariff, noNote, unlimited                                                                                                                                   string
	traffLimit, devLimit, resetTr, reissue, tariffBtn, noteBtn                                                                                                                                     string
	delBtn, quick, other                                                                                                                                                                           string
	gbAsk, gbBad, devAsk, devBad, noteAsk, noteBad                                                                                                                                                 string
	askReset, askReissue, askDelete, askDelete2, askTariff                                                                                                                                         string
	reissued, deleted, noTariffs, askGB, askDev, payAmount, ntPaid, ntRefund, ntNewUser, ntNodeUp, ntNodeDown, ntSource, ntDownFor, srcAdmin, srcBot, srcFree, durSecs, durMins, durHours, durDays string

	// Payments.
	payAll, payPaid, payPending, payFailed, payRefunded string
	payCard, payKind, payTariff, payTerm, payPaidAt     string
	payErr, payUser, payBy, payOpenUser, payFilter      string
	kindNew, kindRenew, kindPackage, kindTrial, kindDev string

	// Servers and the overview.
	srvTitle, ovToday string

	// The overview's money and the clients' filters.
	ovRevenue, ovToday2, ovWeek, ovMonth, ovNew, ovSoon       string
	fAll, fSoon, fEnded, fLow, fNoTG, fBlocked, fShown, fNone string
}

var admRU = admWords{
	cmdAdmin: "Администрирование", title: "Администрирование", notAdmin: "Эта команда недоступна.",
	unavailable: "Панель сейчас недоступна. Попробуйте позже.", failed: "Не получилось. Попробуйте ещё раз.",
	notFound:  "Подписка не найдена.",
	failOld:   "Панель ещё не знает этой функции. Обновите панель до версии бота.",
	failInput: "Панель не приняла эти данные. Проверьте значение и попробуйте ещё раз.",
	overview:  "📊 Сводка", users: "👥 Клиенты", nodes: "🖥 Серверы", payments: "💳 Платежи",
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
	bcGone:   "Текст рассылки потерялся (бот перезапускался). Пришлите его заново.",
	stActive: "Активна", stExpiring: "Скоро закончится", stExpired: "Закончилась", stLimited: "Лимит трафика", stDisabled: "Заблокирована",
	pPaid: "Оплачен", pPending: "Ожидает оплаты", pFailed: "Ошибка", pExpired: "Истёк", pRefunded: "Возврат",
	endedToday: "закончилась сегодня", endedAgo: "закончилась %d дн. назад", endsToday: "заканчивается сегодня", endsIn: "ещё %d дн.",
	cardTariff: "Тариф", cardNote: "Заметка", cardSince: "С нами с", noTariff: "без тарифа", noNote: "нет", unlimited: "без лимита",
	traffLimit: "📶 Лимит трафика", devLimit: "📱 Лимит устройств", resetTr: "♻️ Сбросить трафик", reissue: "🔑 Новая ссылка",
	tariffBtn: "🏷 Тариф", noteBtn: "📝 Заметка", delBtn: "🗑 Удалить", quick: "Быстро", other: "Другое…",
	gbAsk: "Лимит трафика в ГБ? Число от 0 до %d, 0 снимает лимит.", gbBad: "Нужно число ГБ от 0 до %d.",
	devAsk: "Лимит устройств? Число от 0 до %d, 0 снимает лимит.", devBad: "Нужно число от 0 до %d.",
	noteAsk: "Пришлите заметку (до %d символов). Слово «-» очищает её.", noteBad: "Заметка длиннее %d символов. Пришлите короче.",
	askReset:   "Сбросить счётчик трафика у подписки «%s»?",
	askReissue: "Выдать подписке «%s» новую ссылку? Старая перестанет работать, клиенту придётся обновить её в приложении.",
	askDelete:  "Удалить подписку «%s»? Это необратимо.", askDelete2: "Точно удалить «%s»? Подписку и её устройства вернуть нельзя.",
	askTariff: "Применить тариф «%s» к подписке «%s»? Лимиты станут как в тарифе, срок начнётся заново.",
	reissued:  "Новая ссылка выдана. Клиент увидит её в боте.", deleted: "Подписка удалена.", noTariffs: "Тарифов нет.",
	payAll: "Все", payPaid: "✅ Оплаченные", payPending: "⏳ Ожидают", payFailed: "❌ Ошибки", payRefunded: "↩️ Возвраты",
	payCard: "Платёж", payKind: "Что куплено", payTariff: "Тариф", payTerm: "Срок", payPaidAt: "Оплачен",
	payErr: "Ошибка", payUser: "Клиент", payBy: "Способ", payOpenUser: "👤 Открыть клиента", payFilter: "Показано",
	kindNew: "новая подписка", kindRenew: "продление", kindPackage: "пакет трафика", kindTrial: "пробный период", kindDev: "доп. устройство",
	ovRevenue: "Выручка", ovToday2: "Сегодня", ovWeek: "7 дней", ovMonth: "Месяц", ovNew: "Новых за сутки", ovSoon: "Скоро закончатся",
	fAll: "Все", fSoon: "⏳ Скоро", fEnded: "🔴 Закончились", fLow: "📦 Трафик 90%+", fNoTG: "🔗 Без Telegram", fBlocked: "⛔ Заблокированы",
	fShown: "Показаны", fNone: "Под этот фильтр никто не подходит.",
	srvTitle: "Серверы", ovToday: "Сегодня", payAmount: "Сумма",
	ntPaid: "Новая оплата", ntRefund: "Возврат платежа", ntNewUser: "Новый клиент", ntNodeUp: "Сервер снова работает", ntNodeDown: "Сервер недоступен",
	ntSource: "Откуда", ntDownFor: "Не работал", srcAdmin: "создан в панели", srcBot: "из бота", srcFree: "бесплатный тариф",
	durSecs: "меньше минуты", durMins: "%d мин.", durHours: "%d ч.", durDays: "%d дн.",
	askGB: "Установить подписке «%s» лимит трафика: %s ГБ?", askDev: "Установить подписке «%s» лимит устройств: %s?",
}

var admEN = admWords{
	cmdAdmin: "Administration", title: "Administration", notAdmin: "This command is not available.",
	unavailable: "The panel is unavailable right now. Try again later.", failed: "That did not work. Try again.",
	notFound:  "Subscription not found.",
	failOld:   "The panel does not know this function yet. Update the panel to the bot's version.",
	failInput: "The panel did not accept that. Check the value and try again.",
	overview:  "📊 Overview", users: "👥 Clients", nodes: "🖥 Servers", payments: "💳 Payments",
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
	bcGone:   "The broadcast text was lost (the bot restarted). Send it again.",
	stActive: "Active", stExpiring: "Expiring soon", stExpired: "Expired", stLimited: "Traffic limit hit", stDisabled: "Blocked",
	pPaid: "Paid", pPending: "Awaiting payment", pFailed: "Failed", pExpired: "Expired", pRefunded: "Refunded",
	endedToday: "ended today", endedAgo: "ended %d d ago", endsToday: "ends today", endsIn: "%d d left",
	cardTariff: "Plan", cardNote: "Note", cardSince: "Client since", noTariff: "no plan", noNote: "none", unlimited: "unlimited",
	traffLimit: "📶 Traffic limit", devLimit: "📱 Device limit", resetTr: "♻️ Reset traffic", reissue: "🔑 New link",
	tariffBtn: "🏷 Plan", noteBtn: "📝 Note", delBtn: "🗑 Delete", quick: "Quick", other: "Other…",
	gbAsk: "Traffic limit in GB? A number from 0 to %d; 0 removes the limit.", gbBad: "Enter a number of GB from 0 to %d.",
	devAsk: "Device limit? A number from 0 to %d; 0 removes the limit.", devBad: "Enter a number from 0 to %d.",
	noteAsk: "Send the note (up to %d characters). The word “-” clears it.", noteBad: "The note is longer than %d characters. Send a shorter one.",
	askReset:   "Reset the traffic counter of subscription “%s”?",
	askReissue: "Give subscription “%s” a new link? The old one stops working and the client must update it in their app.",
	askDelete:  "Delete subscription “%s”? This cannot be undone.", askDelete2: "Really delete “%s”? The subscription and its devices cannot be brought back.",
	askTariff: "Apply plan “%s” to subscription “%s”? The limits become the plan's and the term starts over.",
	reissued:  "A new link was issued. The client sees it in the bot.", deleted: "The subscription is deleted.", noTariffs: "No plans.",
	payAll: "All", payPaid: "✅ Paid", payPending: "⏳ Pending", payFailed: "❌ Failed", payRefunded: "↩️ Refunds",
	payCard: "Payment", payKind: "Bought", payTariff: "Plan", payTerm: "Term", payPaidAt: "Paid",
	payErr: "Error", payUser: "Client", payBy: "Method", payOpenUser: "👤 Open client", payFilter: "Showing",
	kindNew: "new subscription", kindRenew: "renewal", kindPackage: "traffic package", kindTrial: "trial", kindDev: "extra device",
	ovRevenue: "Revenue", ovToday2: "Today", ovWeek: "7 days", ovMonth: "Month", ovNew: "New in 24h", ovSoon: "Ending soon",
	fAll: "All", fSoon: "⏳ Soon", fEnded: "🔴 Ended", fLow: "📦 Traffic 90%+", fNoTG: "🔗 No Telegram", fBlocked: "⛔ Blocked",
	fShown: "Showing", fNone: "Nobody matches this filter.",
	srvTitle: "Servers", ovToday: "Today", payAmount: "Amount",
	ntPaid: "New payment", ntRefund: "Payment refunded", ntNewUser: "New client", ntNodeUp: "Server is back", ntNodeDown: "Server is down",
	ntSource: "Source", ntDownFor: "Was down for", srcAdmin: "made in the panel", srcBot: "from the bot", srcFree: "free plan",
	durSecs: "under a minute", durMins: "%d min", durHours: "%d h", durDays: "%d d",
	askGB: "Set subscription “%s” traffic limit to %s GB?", askDev: "Set subscription “%s” device limit to %s?",
}

func admWordsFor(lang string) *admWords { return textutil.ByLang(lang, &admRU, &admEN) }
