package tgbot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"mikan/internal/panel/model"
	"mikan/internal/textutil"
	"mikan/internal/tgapi"
)

// The shop in /admin: promo codes, tariffs and the top lists. The buttons:
//
//	aQ:<page>             the promo codes with their totals
//	aq:<id>               a code's card       aq:<id>:e   turn on or off (asks)
//	aq:<id>:u:<page>      its activations     aq:<id>:d   remove (asks twice)
//	aq:<id>:z:<k>         the confirmed one: k is e1, e0, d
//	aW[:t[:v[:u[:d[:code[:y]]]]]]  a new code, step by step: the type (d days, t traffic in GB,
//	                      p percent, r rubles, s Stars), the value, the uses (0 any), the days it
//	                      lives (0 forever), the code ("-" a random one), y creates it
//	aF                    the tariffs         aF:<id>     a tariff's card
//	aF:<id>:s             on sale or off (asks)  aF:<id>:p|x  ask for the price in ₽ or ⭐
//	aF:<id>:P|X:<n>       the price asked     aF:<id>:z:<k>  the confirmed one: s1, s0, P<n>, X<n>
//	aY:<k>                a top list: t traffic, m money, s shared subscriptions

// Words of the shop's screens.
type shopWords struct {
	promos, tariffs, tops                                               string
	stCodes, stActive, stUses, stDays, stBytes, stOrders                string
	noPromos, newPromo, promoBack                                       string
	tDays, tTraffic, tPercent, tFixed, tStars                           string
	lType, lUses, lPerUser, lStarts, lEnds, lOnly, lTariffs, lCreated   string
	forever, unlimited, firstOnly, newOnly, anyTariff, nTariffs         string
	stOn, stOff, stExpired, stExhausted, stScheduled                    string
	turnOn, turnOff, uses, remove                                       string
	askOn, askOff, askDel, askDel2, changed, removed, created           string
	usesTitle, noUses                                                   string
	wTitle, wStep, wType, wValue, wUses, wTerm, wCode, wAuto, wOwn      string
	wPreview, wCreate, wOnce, wDays, wAnyCur, wRandom                   string
	askDays, askTraffic, askPercent, askRub, askStars, askUses, askCode string
	badNum, badCode, taken                                              string
	tariffsHint, noTariffs, tariffBack, onSale, offSale                 string
	lTerm, lTraffic, lDevices, lSpeed, lPriceRub, lPriceStars, noPrice  string
	lTerms, lSubs, sellOn, sellOff, priceRub, priceStars                string
	askSellOn, askSellOff, askPriceRub, askPriceStars, askPrice         string
	askPriceOff, termsNote, mbit, noLimit, daysN                        string
	tabTraffic, tabMoney, tabShare, hintTraffic, hintMoney, hintShare   string
	noTop, noShare, ipsDay, overLimit, devicesW                         string
}

var shopRU = shopWords{
	promos: "🎟 Промокоды", tariffs: "🏷 Тарифы", tops: "🏆 Топы",
	stCodes: "Всего кодов", stActive: "Действуют", stUses: "Активаций", stDays: "Подарено дней", stBytes: "Подарено трафика",
	stOrders: "Покупок со скидкой",
	noPromos: "Промокодов пока нет. Создайте первый.", newPromo: "➕ Новый промокод", promoBack: "← Промокоды",
	tDays: "Дни подписки", tTraffic: "Трафик", tPercent: "Скидка в процентах", tFixed: "Скидка в рублях", tStars: "Скидка в Stars",
	lType: "Что даёт", lUses: "Активации", lPerUser: "На одного клиента", lStarts: "Начало", lEnds: "Действует до",
	lOnly: "Условие", lTariffs: "Тарифы", lCreated: "Создан",
	forever: "бессрочно", unlimited: "без лимита", firstOnly: "только первая покупка", newOnly: "только новые клиенты",
	anyTariff: "любой", nTariffs: "выбрано: %d",
	stOn: "🟢 Действует", stOff: "⏸ Выключен", stExpired: "⌛ Срок вышел", stExhausted: "🔴 Активации закончились", stScheduled: "🕒 Ещё не начался",
	turnOn: "🟢 Включить", turnOff: "⏸ Выключить", uses: "📜 Активации", remove: "🗑 Удалить",
	askOn:   "Включить промокод %s? Его снова можно будет активировать.",
	askOff:  "Выключить промокод %s? Активировать его больше не получится.",
	askDel:  "Удалить промокод %s?",
	askDel2: "Точно удалить %s? Это необратимо; уже выданные по нему дни и скидки останутся у клиентов.",
	changed: "Готово.", removed: "Промокод удалён.", created: "Промокод создан. Отправьте его клиентам.",
	usesTitle: "Активации", noUses: "Этот код ещё никто не активировал.",
	wTitle: "Новый промокод", wStep: "Шаг %d из 5", wType: "Что даёт промокод?", wValue: "Сколько?",
	wUses: "Сколько раз его можно активировать?", wTerm: "Сколько он будет действовать?", wCode: "Какой будет код?",
	wAuto: "🎲 Случайный", wOwn: "✏️ Свой", wPreview: "Проверьте и создайте", wCreate: "✅ Создать", wOnce: "1 раз",
	wDays: "%d дн.", wAnyCur: "на любой заказ", wRandom: "случайный",
	askDays:     "Пришлите число дней (1–%d).",
	askTraffic:  "Пришлите объём в GB (1–%d).",
	askPercent:  "Пришлите скидку в процентах (1–100).",
	askRub:      "Пришлите сумму скидки в рублях (1–%d).",
	askStars:    "Пришлите скидку в Stars (1–%d).",
	askUses:     "Пришлите, сколько раз код можно активировать (1–%d), или 0 — без лимита.",
	askCode:     "Пришлите код: латинские буквы, цифры, «_» или «-», до %d символов.",
	badNum:      "Нужно число от %d до %d.",
	badCode:     "Код не подошёл: только A–Z, 0–9, «_» и «-», до %d символов.",
	taken:       "Не получилось: такой код уже есть. Выберите другой.",
	tariffsHint: "Выберите тариф.", noTariffs: "Тарифов нет: создайте их в панели.", tariffBack: "← Тарифы",
	onSale: "🟢 В продаже", offSale: "⏸ Снят с продажи",
	lTerm: "Срок", lTraffic: "Трафик", lDevices: "Устройства", lSpeed: "Скорость", lPriceRub: "Цена", lPriceStars: "Цена в Stars",
	noPrice: "не задана", lTerms: "Ещё сроков", lSubs: "Подписок на тарифе",
	sellOn: "🟢 Вернуть в продажу", sellOff: "⏸ Снять с продажи", priceRub: "💰 Цена в ₽", priceStars: "⭐ Цена в Stars",
	askSellOn:     "Вернуть тариф «%s» в продажу? Его снова смогут купить в боте.",
	askSellOff:    "Снять тариф «%s» с продажи? Новые покупки прекратятся, текущие подписки продолжат работать.",
	askPriceRub:   "Пришлите цену в рублях (0–%d), можно с копейками: 299.90. 0 — убрать цену.",
	askPriceStars: "Пришлите цену в Stars (0–%d). 0 — убрать цену.",
	askPrice:      "Поставить тарифу «%s» цену %s?",
	askPriceOff:   "Убрать у тарифа «%s» цену в %s? Купить его этой валютой будет нельзя.",
	termsNote:     "Меняется цена основного срока; остальные сроки правятся в панели.",
	mbit:          "%d Мбит/с", noLimit: "без ограничений", daysN: "%d дн.",
	tabTraffic: "📶 Трафик", tabMoney: "💰 Оплаты", tabShare: "🕵️ Шаринг",
	hintTraffic: "Больше всего трафика за текущий период.",
	hintMoney:   "Больше всего оплат за всё время.",
	hintShare:   "Похоже на общую подписку: устройства заходили с %d+ разных IP за сутки или их больше лимита.",
	noTop:       "Пока никого.", noShare: "Подозрительных подписок нет. 👌",
	ipsDay: "IP за сутки", overLimit: "сверх лимита", devicesW: "устр.",
}

var shopEN = shopWords{
	promos: "🎟 Promo codes", tariffs: "🏷 Tariffs", tops: "🏆 Tops",
	stCodes: "Codes", stActive: "Active", stUses: "Activations", stDays: "Days given", stBytes: "Traffic given",
	stOrders: "Discounted purchases",
	noPromos: "No promo codes yet. Create the first one.", newPromo: "➕ New promo code", promoBack: "← Promo codes",
	tDays: "Subscription days", tTraffic: "Traffic", tPercent: "Percent off", tFixed: "Rubles off", tStars: "Stars off",
	lType: "Gives", lUses: "Activations", lPerUser: "Per client", lStarts: "Starts", lEnds: "Valid until",
	lOnly: "Condition", lTariffs: "Tariffs", lCreated: "Created",
	forever: "forever", unlimited: "no limit", firstOnly: "first purchase only", newOnly: "new clients only",
	anyTariff: "any", nTariffs: "%d chosen",
	stOn: "🟢 Active", stOff: "⏸ Off", stExpired: "⌛ Expired", stExhausted: "🔴 Used up", stScheduled: "🕒 Not started yet",
	turnOn: "🟢 Turn on", turnOff: "⏸ Turn off", uses: "📜 Activations", remove: "🗑 Remove",
	askOn:   "Turn promo code %s on? It can be activated again.",
	askOff:  "Turn promo code %s off? Nobody will be able to activate it.",
	askDel:  "Remove promo code %s?",
	askDel2: "Remove %s for good? The days and discounts it gave stay with the clients.",
	changed: "Done.", removed: "Promo code removed.", created: "Promo code created. Send it to your clients.",
	usesTitle: "Activations", noUses: "Nobody has activated this code yet.",
	wTitle: "New promo code", wStep: "Step %d of 5", wType: "What does the code give?", wValue: "How much?",
	wUses: "How many times can it be activated?", wTerm: "How long is it valid?", wCode: "What is the code?",
	wAuto: "🎲 Random", wOwn: "✏️ My own", wPreview: "Check and create", wCreate: "✅ Create", wOnce: "once",
	wDays: "%d d", wAnyCur: "any order", wRandom: "random",
	askDays:     "Send the number of days (1–%d).",
	askTraffic:  "Send the amount in GB (1–%d).",
	askPercent:  "Send the discount in percent (1–100).",
	askRub:      "Send the discount in rubles (1–%d).",
	askStars:    "Send the discount in Stars (1–%d).",
	askUses:     "Send how many times the code can be activated (1–%d), or 0 for no limit.",
	askCode:     "Send the code: Latin letters, digits, “_” or “-”, up to %d characters.",
	badNum:      "A number from %d to %d is needed.",
	badCode:     "That code won't do: only A–Z, 0–9, “_” and “-”, up to %d characters.",
	taken:       "That did not work: the code is taken. Pick another.",
	tariffsHint: "Pick a tariff.", noTariffs: "No tariffs: create them in the panel.", tariffBack: "← Tariffs",
	onSale: "🟢 On sale", offSale: "⏸ Off sale",
	lTerm: "Term", lTraffic: "Traffic", lDevices: "Devices", lSpeed: "Speed", lPriceRub: "Price", lPriceStars: "Price in Stars",
	noPrice: "not set", lTerms: "More terms", lSubs: "Subscriptions on it",
	sellOn: "🟢 Put on sale", sellOff: "⏸ Take off sale", priceRub: "💰 Price in ₽", priceStars: "⭐ Price in Stars",
	askSellOn:     "Put tariff “%s” on sale? It can be bought in the bot again.",
	askSellOff:    "Take tariff “%s” off sale? New purchases stop; current subscriptions keep working.",
	askPriceRub:   "Send the price in rubles (0–%d), kopecks allowed: 299.90. 0 removes the price.",
	askPriceStars: "Send the price in Stars (0–%d). 0 removes the price.",
	askPrice:      "Set the price of tariff “%s” to %s?",
	askPriceOff:   "Remove the %[2]s price of tariff “%[1]s”? It can't be bought in that currency then.",
	termsNote:     "This changes the main term's price; other terms are edited in the panel.",
	mbit:          "%d Mbit/s", noLimit: "no limit", daysN: "%d d",
	tabTraffic: "📶 Traffic", tabMoney: "💰 Payments", tabShare: "🕵️ Sharing",
	hintTraffic: "The most traffic in the current period.",
	hintMoney:   "The most paid, all time.",
	hintShare:   "Looks shared: the devices came from %d+ different IPs in a day, or there are more than the limit.",
	noTop:       "Nobody yet.", noShare: "No suspicious subscriptions. 👌",
	ipsDay: "IPs a day", overLimit: "over the limit", devicesW: "dev.",
}

func shopWordsFor(lang string) *shopWords { return textutil.ByLang(lang, &shopRU, &shopEN) }

// shareIPs is the number of addresses a day the panel counts as a shared subscription.
const shareIPs = 3

// promoTypeLetter and promoTypeOf map a code's type (and currency) to the wizard's letter.
func promoTypeOf(letter string) (typ, cur string, ok bool) {
	switch letter {
	case "d":
		return "days", "", true
	case "t":
		return "traffic", "", true
	case "p":
		return "percent", "", true
	case "r":
		return "fixed", "RUB", true
	case "s":
		return "fixed", "XTR", true
	}
	return "", "", false
}

// wizardMax is the largest value the wizard takes for the type letter, in its own unit.
func wizardMax(letter string) int64 {
	switch letter {
	case "d":
		return MaxPromoDays
	case "t":
		return MaxPromoTraffic
	case "p":
		return 100
	case "r":
		return MaxPriceRub
	case "s":
		return MaxPriceStars
	}
	return 0
}

// promoInputOf is the code the wizard's steps describe; v is in the wizard's unit.
func promoInputOf(letter string, v, uses, days int64, code string) (AdminPromoInput, bool) {
	typ, cur, ok := promoTypeOf(letter)
	if !ok || v < 1 || v > wizardMax(letter) || uses < 0 || uses > MaxPromoUses || days < 0 || days > MaxPromoDays {
		return AdminPromoInput{}, false
	}
	in := AdminPromoInput{Type: typ, Currency: cur, Value: v, MaxUses: uses, Days: days}
	switch letter {
	case "t":
		in.Value = v << 30
	case "r":
		in.Value = v * 100
	}
	if code != "-" {
		in.Code = code
	}
	return in, true
}

// promoValue is what a code gives, in a few words: «+30 дн.», «+50.0 GB», «−20%», «−100.00 ₽».
func promoValue(sw *shopWords, p AdminPromo) string {
	switch p.Type {
	case "days":
		return "+" + fmt.Sprintf(sw.daysN, p.Value)
	case "traffic":
		return "+" + gib(p.Value)
	case "percent":
		return fmt.Sprintf("−%d%%", p.Value)
	case "fixed":
		return "−" + money(p.Value, p.Currency)
	}
	return strconv.FormatInt(p.Value, 10)
}

func promoTypeWord(sw *shopWords, p AdminPromo) string {
	switch {
	case p.Type == "days":
		return sw.tDays
	case p.Type == "traffic":
		return sw.tTraffic
	case p.Type == "percent":
		return sw.tPercent
	case p.Type == "fixed" && p.Currency == "XTR":
		return sw.tStars
	case p.Type == "fixed":
		return sw.tFixed
	}
	return p.Type
}

// promoState is the code's state as a dot and a word.
func promoState(sw *shopWords, p AdminPromo, now time.Time) (dot, word string) {
	n := now.Unix()
	switch {
	case !p.Enabled:
		return "⏸", sw.stOff
	case p.EndsAt > 0 && n >= p.EndsAt:
		return "⌛", sw.stExpired
	case p.MaxUses > 0 && p.Uses >= p.MaxUses:
		return "🔴", sw.stExhausted
	case p.StartsAt > 0 && n < p.StartsAt:
		return "🕒", sw.stScheduled
	}
	return "🟢", sw.stOn
}

func usesText(sw *shopWords, uses, max int64) string {
	if max <= 0 {
		return fmt.Sprintf("%d · %s", uses, sw.unlimited)
	}
	return fmt.Sprintf("%d / %d", uses, max)
}

func (b *Bot) promoList(ctx context.Context, adm Admin, chat int64, arg string, aw *admWords, sw *shopWords, pre string,
	fail func(error) (string, *tgapi.Keyboard), home []tgapi.Button) (string, *tgapi.Keyboard) {
	page := max(int(atoi(arg)), 0)
	st, err := adm.PromoStats(ctx, chat)
	if err != nil {
		return fail(err)
	}
	ps, more, err := adm.Promos(ctx, chat, page)
	if err != nil {
		return fail(err)
	}
	now := b.d.Now()
	stats := block("",
		Row{"🎟", sw.stCodes, strconv.FormatInt(st.Codes, 10)},
		Row{"🟢", sw.stActive, strconv.FormatInt(st.Active, 10)},
		Row{"✅", sw.stUses, strconv.FormatInt(st.Uses, 10)})
	gifts := block("",
		Row{"📅", sw.stDays, strconv.FormatInt(st.Days, 10)},
		Row{"📶", sw.stBytes, gib(st.Bytes)},
		Row{"🛒", sw.stOrders, strconv.FormatInt(st.Orders, 10)})
	parts := []string{head("🎟", sw.promos), stats, gifts}
	var rows [][]tgapi.Button
	for _, p := range ps {
		dot, _ := promoState(sw, p, now)
		label := fmt.Sprintf("%s %s · %s · %s", dot, p.Code, promoValue(sw, p), usesText(sw, p.Uses, p.MaxUses))
		rows = append(rows, []tgapi.Button{{Text: truncate(label, 48), CallbackData: "aq:" + strconv.FormatInt(p.ID, 10)}})
	}
	if len(ps) == 0 && page == 0 {
		parts = append(parts, html.EscapeString(sw.noPromos))
	} else {
		parts = append(parts, html.EscapeString(fmt.Sprintf(aw.page, page+1)))
	}
	var nav []tgapi.Button
	if page > 0 {
		nav = append(nav, tgapi.Button{Text: aw.prev, CallbackData: "aQ:" + strconv.Itoa(page-1)})
	}
	if more {
		nav = append(nav, tgapi.Button{Text: aw.next, CallbackData: "aQ:" + strconv.Itoa(page+1)})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, []tgapi.Button{clr(tgapi.StyleSuccess, tgapi.Button{Text: sw.newPromo, CallbackData: "aW"})}, home)
	return pre + join(parts...), &tgapi.Keyboard{InlineKeyboard: rows}
}

func (b *Bot) promoCardOf(p AdminPromo, aw *admWords, sw *shopWords, pre string) (string, *tgapi.Keyboard) {
	now := b.d.Now()
	_, state := promoState(sw, p, now)
	gives := block("", Row{"🎁", sw.lType, promoTypeWord(sw, p)}, Row{"✨", "", promoValue(sw, p)})
	uses := html.EscapeString("✅ "+sw.lUses+": ") + "<b>" + html.EscapeString(usesText(sw, p.Uses, p.MaxUses)) + "</b>"
	if p.MaxUses > 0 {
		uses += "\n<code>" + bar(p.Uses, p.MaxUses) + "</code>"
	}
	ends := sw.forever
	if p.EndsAt > 0 {
		at := time.Unix(p.EndsAt, 0)
		ends = at.Format("02.01.2006")
		if at.After(now) {
			ends += " · " + left(aw, at, now)
		}
	}
	term := []Row{}
	if p.StartsAt > 0 {
		term = append(term, Row{"🕒", sw.lStarts, time.Unix(p.StartsAt, 0).Format("02.01.2006")})
	}
	term = append(term, Row{"📅", sw.lEnds, ends}, Row{"👤", sw.lPerUser, strconv.FormatInt(p.PerUser, 10)})
	if p.FirstOnly {
		term = append(term, Row{"🛒", sw.lOnly, sw.firstOnly})
	}
	if p.NewOnly {
		term = append(term, Row{"🆕", sw.lOnly, sw.newOnly})
	}
	tariffs := sw.anyTariff
	if p.Tariffs > 0 {
		tariffs = fmt.Sprintf(sw.nTariffs, p.Tariffs)
	}
	term = append(term, Row{"🏷", sw.lTariffs, tariffs})
	if p.CreatedAt > 0 {
		term = append(term, Row{"🗓", sw.lCreated, time.Unix(p.CreatedAt, 0).Format("02.01.2006")})
	}
	parts := []string{head("🎟", p.Code), "<b>" + html.EscapeString(state) + "</b>", gives, "<blockquote>" + uses + "</blockquote>", block("", term...)}
	if p.Name != "" && p.Name != p.Code {
		parts = append(parts[:1], append([]string{"<i>" + html.EscapeString(truncate(p.Name, 100)) + "</i>"}, parts[1:]...)...)
	}
	idS := strconv.FormatInt(p.ID, 10)
	toggle := clr(tgapi.StyleSuccess, tgapi.Button{Text: sw.turnOn, CallbackData: "aq:" + idS + ":e"})
	if p.Enabled {
		toggle = tgapi.Button{Text: sw.turnOff, CallbackData: "aq:" + idS + ":e"}
	}
	return pre + join(parts...), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{
		{clr(tgapi.StylePrimary, tgapi.Button{Text: sw.uses, CallbackData: "aq:" + idS + ":u:0"})},
		{toggle, clr(tgapi.StyleDanger, tgapi.Button{Text: sw.remove, CallbackData: "aq:" + idS + ":d"})},
		{{Text: sw.promoBack, CallbackData: "aQ:0"}},
	}}
}

func (b *Bot) promoScreen(ctx context.Context, adm Admin, chat int64, arg string, aw *admWords, sw *shopWords, pre string,
	fail func(error) (string, *tgapi.Keyboard), home []tgapi.Button) (string, *tgapi.Keyboard) {
	idS, rest, _ := strings.Cut(arg, ":")
	id := atoi(idS)
	if id <= 0 {
		return fail(model.ErrNotFound)
	}
	act, val, _ := strings.Cut(rest, ":")
	p, err := adm.Promo(ctx, chat, id)
	if err != nil {
		return fail(err)
	}
	back := "aq:" + idS
	switch act {
	case "e":
		format, k := sw.askOn, "e1"
		if p.Enabled {
			format, k = sw.askOff, "e0"
		}
		return pre + html.EscapeString(fmt.Sprintf(format, p.Code)), confirm(aw, back+":z:"+k, back)
	case "d":
		if val == "2" {
			return pre + html.EscapeString(fmt.Sprintf(sw.askDel2, p.Code)), confirm(aw, back+":z:d", back)
		}
		return pre + html.EscapeString(fmt.Sprintf(sw.askDel, p.Code)), confirm(aw, back+":d:2", back)
	case "z":
		switch val {
		case "e1", "e0":
			p, err = adm.SetPromoEnabled(ctx, chat, id, val == "e1")
			if err != nil {
				return fail(err)
			}
			return b.promoCardOf(p, aw, sw, html.EscapeString(sw.changed)+"\n\n")
		case "d":
			if err := adm.DeletePromo(ctx, chat, id); err != nil {
				return fail(err)
			}
			return b.promoList(ctx, adm, chat, "0", aw, sw, html.EscapeString(sw.removed)+"\n\n", fail, home)
		}
	case "u":
		page := max(int(atoi(val)), 0)
		us, more, err := adm.PromoUses(ctx, chat, id, page)
		if err != nil {
			return fail(err)
		}
		parts := []string{head("📜", sw.usesTitle+" · "+p.Code)}
		var rows [][]tgapi.Button
		if len(us) == 0 && page == 0 {
			parts = append(parts, html.EscapeString(sw.noUses))
		} else {
			var lines []Row
			for _, u := range us {
				who := u.User
				if who == "" && u.TgID != 0 {
					who = strconv.FormatInt(u.TgID, 10)
				}
				lines = append(lines, Row{useDot(u.Status), time.Unix(u.At, 0).Format("02.01 15:04") + " · " + who, useGift(sw, u)})
				if u.UserID != 0 && len(rows) < 8 {
					rows = append(rows, []tgapi.Button{{Text: truncate("👤 "+who, 40), CallbackData: "ak:" + strconv.FormatInt(u.UserID, 10)}})
				}
			}
			parts = append(parts, block("", lines...), html.EscapeString(fmt.Sprintf(aw.page, page+1)))
		}
		var nav []tgapi.Button
		if page > 0 {
			nav = append(nav, tgapi.Button{Text: aw.prev, CallbackData: back + ":u:" + strconv.Itoa(page-1)})
		}
		if more {
			nav = append(nav, tgapi.Button{Text: aw.next, CallbackData: back + ":u:" + strconv.Itoa(page+1)})
		}
		if len(nav) > 0 {
			rows = append(rows, nav)
		}
		rows = append(rows, []tgapi.Button{{Text: aw.back, CallbackData: back}})
		return pre + join(parts...), &tgapi.Keyboard{InlineKeyboard: rows}
	}
	return b.promoCardOf(p, aw, sw, pre)
}

func useDot(status string) string {
	switch status {
	case "refunded", "reverted", "canceled", "cancelled", "expired":
		return "↩️"
	case "reserved", "pending":
		return "⏳"
	}
	return "✅"
}

func useGift(sw *shopWords, u AdminPromoUse) string {
	switch {
	case u.Days > 0:
		return "+" + fmt.Sprintf(sw.daysN, u.Days)
	case u.Bytes > 0:
		return "+" + gib(u.Bytes)
	case u.Discount > 0:
		return "−" + money(u.Discount, u.Currency)
	}
	return ""
}

// promoWizard draws a step of a new code. The steps live in the button's data alone, so a
// restart of the bot loses nothing.
func (b *Bot) promoWizard(ctx context.Context, adm Admin, chat int64, arg string, aw *admWords, sw *shopWords, pre string,
	fail func(error) (string, *tgapi.Keyboard)) (string, *tgapi.Keyboard) {
	var st []string
	if arg != "" {
		st = strings.Split(arg, ":")
	}
	cancel := []tgapi.Button{clr(tgapi.StyleDanger, tgapi.Button{Text: aw.cancel, CallbackData: "aQ:0"})}
	at := "aW"
	if len(st) > 0 {
		at += ":" + strings.Join(st, ":")
	}
	step := func(n int, ask string, rows ...[]tgapi.Button) (string, *tgapi.Keyboard) {
		t := head("🎟", sw.wTitle) + "\n" + html.EscapeString(fmt.Sprintf(sw.wStep, n)) + "\n\n" + "<b>" + html.EscapeString(ask) + "</b>"
		if summary := wizardSummary(sw, st); summary != "" {
			t += "\n\n" + summary
		}
		return pre + t, &tgapi.Keyboard{InlineKeyboard: append(rows, cancel)}
	}
	opt := func(text, v string) tgapi.Button { return tgapi.Button{Text: text, CallbackData: at + ":" + v} }
	switch len(st) {
	case 0:
		return step(1, sw.wType,
			[]tgapi.Button{opt("📅 "+sw.tDays, "d"), opt("📶 "+sw.tTraffic, "t")},
			[]tgapi.Button{opt("％ "+sw.tPercent, "p")},
			[]tgapi.Button{opt("₽ "+sw.tFixed, "r"), opt("⭐ "+sw.tStars, "s")})
	}
	if st[0] == "?" { // aW:?:<kind>:<steps>: ask for a value the presets lack
		return b.wizardAsk(chat, st[1:], aw, sw, pre)
	}
	letter := st[0]
	if _, _, ok := promoTypeOf(letter); !ok {
		return fail(model.ErrAdminInput)
	}
	switch len(st) {
	case 1:
		var presets []string
		switch letter {
		case "d":
			presets = []string{"7", "14", "30", "90"}
		case "t":
			presets = []string{"10", "50", "100", "500"}
		case "p":
			presets = []string{"10", "20", "30", "50"}
		case "r":
			presets = []string{"50", "100", "200", "500"}
		case "s":
			presets = []string{"25", "50", "100", "250"}
		}
		var row []tgapi.Button
		for _, v := range presets {
			row = append(row, opt(wizardValue(sw, letter, atoi(v)), v))
		}
		return step(2, sw.wValue, row, []tgapi.Button{clr(tgapi.StylePrimary, tgapi.Button{Text: sw.wOwn, CallbackData: "aW:?:v:" + letter})})
	case 2:
		return step(3, sw.wUses,
			[]tgapi.Button{opt("♾ "+sw.unlimited, "0"), opt(sw.wOnce, "1")},
			[]tgapi.Button{opt("10", "10"), opt("50", "50"), opt("100", "100"), opt("1000", "1000")},
			[]tgapi.Button{clr(tgapi.StylePrimary, tgapi.Button{Text: sw.wOwn, CallbackData: "aW:?:u:" + strings.Join(st, ":")})})
	case 3:
		return step(4, sw.wTerm,
			[]tgapi.Button{opt("♾ "+sw.forever, "0")},
			[]tgapi.Button{opt(fmt.Sprintf(sw.wDays, 7), "7"), opt(fmt.Sprintf(sw.wDays, 30), "30"), opt(fmt.Sprintf(sw.wDays, 90), "90")})
	case 4:
		return step(5, sw.wCode,
			[]tgapi.Button{opt(sw.wAuto, "-"), clr(tgapi.StylePrimary, tgapi.Button{Text: sw.wOwn, CallbackData: "aW:?:c:" + strings.Join(st, ":")})})
	}
	in, ok := promoInputOf(letter, atoi(st[1]), atoi(st[2]), atoi(st[3]), st[4])
	if !ok || (st[4] != "-" && cleanCode(st[4]) == "") || len(st) > 6 || (len(st) == 6 && st[5] != "y") {
		return fail(model.ErrAdminInput)
	}
	if len(st) == 5 {
		t := head("🎟", sw.wTitle) + "\n\n<b>" + html.EscapeString(sw.wPreview) + "</b>\n\n" + wizardSummary(sw, st)
		return pre + t, &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{
			{clr(tgapi.StyleSuccess, tgapi.Button{Text: sw.wCreate, CallbackData: at + ":y"}), clr(tgapi.StyleDanger, tgapi.Button{Text: aw.cancel, CallbackData: "aQ:0"})},
		}}
	}
	p, err := adm.CreatePromo(ctx, chat, in)
	if errors.Is(err, model.ErrAdminInput) {
		return b.promoWizard(ctx, adm, chat, strings.Join(st[:4], ":"), aw, sw, html.EscapeString(sw.taken)+"\n\n", fail)
	}
	if err != nil {
		return fail(err)
	}
	return b.promoCardOf(p, aw, sw, html.EscapeString(sw.created)+"\n\n")
}

// wizardAsk asks for a value the wizard's buttons lack and waits for it: k is v (the value),
// u (the uses) or c (the code), st the steps before it.
func (b *Bot) wizardAsk(chat int64, st []string, aw *admWords, sw *shopWords, pre string) (string, *tgapi.Keyboard) {
	if len(st) < 2 {
		return html.EscapeString(aw.failed), nil
	}
	kind, steps := st[0], strings.Join(st[1:], ":")
	var ask string
	switch kind {
	case "v":
		switch st[1] {
		case "d":
			ask = fmt.Sprintf(sw.askDays, MaxPromoDays)
		case "t":
			ask = fmt.Sprintf(sw.askTraffic, MaxPromoTraffic)
		case "p":
			ask = sw.askPercent
		case "r":
			ask = fmt.Sprintf(sw.askRub, MaxPriceRub)
		case "s":
			ask = fmt.Sprintf(sw.askStars, MaxPriceStars)
		}
	case "u":
		ask = fmt.Sprintf(sw.askUses, MaxPromoUses)
	case "c":
		ask = fmt.Sprintf(sw.askCode, MaxPromoCode)
	}
	if ask == "" {
		return html.EscapeString(aw.failed), nil
	}
	b.setWait(chat, adminState{kind: "promo_" + kind, query: steps})
	return pre + head("🎟", sw.wTitle) + "\n\n" + html.EscapeString(ask), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{
		{{Text: aw.back, CallbackData: "aW:" + steps}},
	}}
}

// wizardValue is a value of the wizard's type letter in its unit, as the buttons show it.
func wizardValue(sw *shopWords, letter string, v int64) string {
	switch letter {
	case "d":
		return fmt.Sprintf(sw.daysN, v)
	case "t":
		return fmt.Sprintf("%d GB", v)
	case "p":
		return fmt.Sprintf("%d%%", v)
	case "r":
		return fmt.Sprintf("%d ₽", v)
	case "s":
		return fmt.Sprintf("%d ⭐", v)
	}
	return strconv.FormatInt(v, 10)
}

// wizardSummary is what the steps so far chose, as a block.
func wizardSummary(sw *shopWords, st []string) string {
	if len(st) == 0 || st[0] == "?" {
		return ""
	}
	typ, cur, ok := promoTypeOf(st[0])
	if !ok {
		return ""
	}
	rows := []Row{{"🎁", sw.lType, promoTypeWord(sw, AdminPromo{Type: typ, Currency: cur})}}
	if len(st) > 1 {
		value := wizardValue(sw, st[0], atoi(st[1]))
		if in, ok := promoInputOf(st[0], atoi(st[1]), 0, 0, "-"); ok {
			value = promoValue(sw, AdminPromo{Type: in.Type, Value: in.Value, Currency: in.Currency})
		}
		rows = append(rows, Row{"✨", "", value})
	}
	if len(st) > 2 {
		rows = append(rows, Row{"✅", sw.lUses, usesText(sw, 0, atoi(st[2]))})
		if atoi(st[2]) <= 0 {
			rows[len(rows)-1].Value = sw.unlimited
		} else {
			rows[len(rows)-1].Value = strconv.FormatInt(atoi(st[2]), 10)
		}
	}
	if len(st) > 3 {
		term := sw.forever
		if d := atoi(st[3]); d > 0 {
			term = fmt.Sprintf(sw.wDays, d)
		}
		rows = append(rows, Row{"📅", sw.lTerm, term})
	}
	if len(st) > 4 {
		code := st[4]
		if code == "-" {
			code = sw.wRandom
		}
		rows = append(rows, Row{"🔑", "", code})
	}
	return block("", rows...)
}

// cleanCode is the code as the panel keeps it, "" when it has what a code may not.
func cleanCode(s string) string {
	c := strings.ToUpper(strings.Join(strings.Fields(s), ""))
	if c == "" || len(c) > MaxPromoCode {
		return ""
	}
	for _, r := range c {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return ""
		}
	}
	return c
}

func priceText(sw *shopWords, amount int64, cur string) string {
	if amount <= 0 {
		return sw.noPrice
	}
	return money(amount, cur)
}

func (b *Bot) tariffList(ctx context.Context, adm Admin, chat int64, aw *admWords, sw *shopWords, pre string,
	fail func(error) (string, *tgapi.Keyboard), home []tgapi.Button) (string, *tgapi.Keyboard) {
	ts, err := adm.Tariffs(ctx, chat)
	if err != nil {
		return fail(err)
	}
	var rows [][]tgapi.Button
	for _, t := range ts {
		dot := "🟢"
		if !t.OnSale {
			dot = "⏸"
		}
		price := ""
		switch {
		case t.PriceRub > 0:
			price = " · " + money(t.PriceRub, "RUB")
		case t.PriceStars > 0:
			price = " · " + money(t.PriceStars, "XTR")
		}
		rows = append(rows, []tgapi.Button{{Text: truncate(dot+" "+t.Name+price, 48), CallbackData: "aF:" + strconv.FormatInt(t.ID, 10)}})
	}
	t := head("🏷", sw.tariffs) + "\n" + html.EscapeString(sw.tariffsHint)
	if len(ts) == 0 {
		t = head("🏷", sw.tariffs) + "\n\n" + html.EscapeString(sw.noTariffs)
	}
	return pre + t, &tgapi.Keyboard{InlineKeyboard: append(rows, home)}
}

func (b *Bot) tariffCardOf(t AdminTariffInfo, aw *admWords, sw *shopWords, pre string) (string, *tgapi.Keyboard) {
	state := sw.onSale
	if !t.OnSale {
		state = sw.offSale
	}
	limit := sw.noLimit
	if t.Limit > 0 {
		limit = gib(t.Limit)
	}
	devices := sw.noLimit
	if t.Devices > 0 {
		devices = strconv.FormatInt(t.Devices, 10)
	}
	speed := sw.noLimit
	if t.Speed > 0 {
		speed = fmt.Sprintf(sw.mbit, t.Speed)
	}
	term := sw.forever
	if t.Days > 0 {
		term = fmt.Sprintf(sw.daysN, t.Days)
	}
	plan := block("", Row{"📅", sw.lTerm, term}, Row{"📶", sw.lTraffic, limit}, Row{"📱", sw.lDevices, devices}, Row{"⚡", sw.lSpeed, speed})
	prices := []Row{{"💰", sw.lPriceRub, priceText(sw, t.PriceRub, "RUB")}, {"⭐", sw.lPriceStars, priceText(sw, t.PriceStars, "XTR")}}
	if t.Terms > 0 {
		prices = append(prices, Row{"➕", sw.lTerms, strconv.Itoa(t.Terms)})
	}
	parts := []string{head("🏷", t.Name), "<b>" + html.EscapeString(state) + "</b>", plan, block("", prices...),
		block("", Row{"👥", sw.lSubs, strconv.FormatInt(t.Users, 10)})}
	if t.Description != "" {
		parts = append(parts, "<i>"+html.EscapeString(truncate(t.Description, 200))+"</i>")
	}
	if t.Terms > 0 {
		parts = append(parts, html.EscapeString("ℹ️ "+sw.termsNote))
	}
	idS := strconv.FormatInt(t.ID, 10)
	sale := clr(tgapi.StyleSuccess, tgapi.Button{Text: sw.sellOn, CallbackData: "aF:" + idS + ":s"})
	if t.OnSale {
		sale = clr(tgapi.StyleDanger, tgapi.Button{Text: sw.sellOff, CallbackData: "aF:" + idS + ":s"})
	}
	return pre + join(parts...), &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{
		tint(tgapi.StylePrimary, tgapi.Button{Text: sw.priceRub, CallbackData: "aF:" + idS + ":p"}, tgapi.Button{Text: sw.priceStars, CallbackData: "aF:" + idS + ":x"}),
		{sale},
		{{Text: sw.tariffBack, CallbackData: "aF"}},
	}}
}

func (b *Bot) tariffScreen(ctx context.Context, adm Admin, chat int64, arg string, aw *admWords, sw *shopWords, pre string,
	fail func(error) (string, *tgapi.Keyboard), home []tgapi.Button) (string, *tgapi.Keyboard) {
	if arg == "" {
		return b.tariffList(ctx, adm, chat, aw, sw, pre, fail, home)
	}
	idS, rest, _ := strings.Cut(arg, ":")
	id := atoi(idS)
	if id <= 0 {
		return fail(model.ErrNotFound)
	}
	act, val, _ := strings.Cut(rest, ":")
	t, err := adm.Tariff(ctx, chat, id)
	if err != nil {
		return fail(err)
	}
	back := "aF:" + idS
	backRow := [][]tgapi.Button{{{Text: aw.back, CallbackData: back}}}
	switch act {
	case "s":
		format, k := sw.askSellOn, "s1"
		if t.OnSale {
			format, k = sw.askSellOff, "s0"
		}
		return pre + html.EscapeString(fmt.Sprintf(format, t.Name)), confirm(aw, back+":z:"+k, back)
	case "p":
		b.setWait(chat, adminState{kind: "tariff_rub", user: id})
		return pre + head("🏷", t.Name) + "\n\n" + html.EscapeString(fmt.Sprintf(sw.askPriceRub, MaxPriceRub)), &tgapi.Keyboard{InlineKeyboard: backRow}
	case "x":
		b.setWait(chat, adminState{kind: "tariff_stars", user: id})
		return pre + head("🏷", t.Name) + "\n\n" + html.EscapeString(fmt.Sprintf(sw.askPriceStars, MaxPriceStars)), &tgapi.Keyboard{InlineKeyboard: backRow}
	case "P", "X":
		n := atoi(val)
		cur := "RUB"
		if act == "X" {
			cur = "XTR"
		}
		if n < 0 || (cur == "RUB" && n > MaxPriceRub*100) || (cur == "XTR" && n > MaxPriceStars) {
			return fail(model.ErrAdminInput)
		}
		q := fmt.Sprintf(sw.askPrice, t.Name, money(n, cur))
		if n == 0 {
			q = fmt.Sprintf(sw.askPriceOff, t.Name, map[string]string{"RUB": "₽", "XTR": "⭐"}[cur])
		}
		return pre + html.EscapeString(q), confirm(aw, back+":z:"+act+strconv.FormatInt(n, 10), back)
	case "z":
		switch {
		case val == "s1" || val == "s0":
			t, err = adm.SetTariffOnSale(ctx, chat, id, val == "s1")
		case strings.HasPrefix(val, "P"):
			t, err = adm.SetTariffPrice(ctx, chat, id, "RUB", atoi(val[1:]))
		case strings.HasPrefix(val, "X"):
			t, err = adm.SetTariffPrice(ctx, chat, id, "XTR", atoi(val[1:]))
		default:
			return b.tariffCardOf(t, aw, sw, pre)
		}
		if err != nil {
			return fail(err)
		}
		return b.tariffCardOf(t, aw, sw, html.EscapeString(sw.changed)+"\n\n")
	}
	return b.tariffCardOf(t, aw, sw, pre)
}

var medals = []string{"🥇", "🥈", "🥉"}

func rank(i int) string {
	if i < len(medals) {
		return medals[i]
	}
	return strconv.Itoa(i+1) + "."
}

func (b *Bot) topScreen(ctx context.Context, adm Admin, chat int64, arg string, aw *admWords, sw *shopWords, pre string,
	fail func(error) (string, *tgapi.Keyboard), home []tgapi.Button) (string, *tgapi.Keyboard) {
	kind, tab, hint := TopTraffic, sw.tabTraffic, sw.hintTraffic
	switch arg {
	case "m":
		kind, tab, hint = TopMoney, sw.tabMoney, sw.hintMoney
	case "s":
		kind, tab, hint = TopShare, sw.tabShare, fmt.Sprintf(sw.hintShare, shareIPs)
	default:
		arg = "t"
	}
	ts, err := adm.Tops(ctx, chat, kind)
	if err != nil {
		return fail(err)
	}
	parts := []string{head("🏆", sw.tops+" · "+plain(tab)), "<i>" + html.EscapeString(hint) + "</i>"}
	var lines []string
	var buttons []tgapi.Button
	for i, t := range ts {
		var value string
		switch kind {
		case TopTraffic:
			value = gib(t.Value)
			if t.Limit > 0 {
				value += " / " + gib(t.Limit)
			}
		case TopMoney:
			var ms []string
			if t.Value > 0 {
				ms = append(ms, money(t.Value, "RUB"))
			}
			if t.Stars > 0 {
				ms = append(ms, money(t.Stars, "XTR"))
			}
			value = strings.Join(ms, " + ")
		case TopShare:
			devices := strconv.FormatInt(t.Devices, 10)
			if t.DeviceLimit > 0 {
				devices += "/" + strconv.FormatInt(t.DeviceLimit, 10)
			}
			value = "📱 " + devices + " · 🌐 " + strconv.FormatInt(t.IPs, 10) + " IP"
		}
		mark := rank(i)
		if kind == TopShare {
			mark = "🟡"
			if t.IPs >= 2*shareIPs || (t.DeviceLimit > 0 && t.Devices > t.DeviceLimit && t.IPs >= shareIPs) {
				mark = "🔴"
			}
		}
		line := html.EscapeString(mark+" "+truncate(t.Name, 32)) + " — <b>" + html.EscapeString(value) + "</b>"
		if kind == TopTraffic && t.Limit > 0 {
			line += "\n<code>" + bar(t.Value, t.Limit) + "</code>"
		}
		if kind == TopShare && t.DeviceLimit > 0 && t.Devices > t.DeviceLimit {
			line += "\n" + html.EscapeString(fmt.Sprintf("   ⚠️ +%d %s %s", t.Devices-t.DeviceLimit, sw.devicesW, sw.overLimit))
		}
		lines = append(lines, line)
		buttons = append(buttons, tgapi.Button{Text: truncate(mark+" "+t.Name, 24), CallbackData: "ak:" + strconv.FormatInt(t.ID, 10)})
	}
	switch {
	case len(ts) == 0 && kind == TopShare:
		parts = append(parts, html.EscapeString(sw.noShare))
	case len(ts) == 0:
		parts = append(parts, html.EscapeString(sw.noTop))
	default:
		parts = append(parts, "<blockquote>"+strings.Join(lines, "\n")+"</blockquote>")
	}
	var rows [][]tgapi.Button
	for i := 0; i < len(buttons); i += 2 {
		rows = append(rows, buttons[i:min(i+2, len(buttons))])
	}
	rows = append(rows, []tgapi.Button{
		{Text: filterMark(arg, "t", sw.tabTraffic), CallbackData: "aY:t"},
		{Text: filterMark(arg, "m", sw.tabMoney), CallbackData: "aY:m"},
		{Text: filterMark(arg, "s", sw.tabShare), CallbackData: "aY:s"},
	}, home)
	return pre + join(parts...), &tgapi.Keyboard{InlineKeyboard: rows}
}

// shopText takes the text an admin was asked for on the shop's screens; false: not one of them.
func (b *Bot) shopText(chat int64, st adminState, text string, sw *shopWords) bool {
	num := func(lo, hi int64) (int64, bool) {
		n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		return n, err == nil && n >= lo && n <= hi
	}
	again := func(data, notice string) bool {
		b.setWait(chat, st)
		b.adminDraw(chat, 0, data, notice)
		return true
	}
	switch st.kind {
	case "promo_v":
		letter := st.query
		n, ok := num(1, wizardMax(letter))
		if !ok {
			return again("aW:"+letter, fmt.Sprintf(sw.badNum, 1, wizardMax(letter)))
		}
		b.adminDraw(chat, 0, "aW:"+letter+":"+strconv.FormatInt(n, 10), "")
	case "promo_u":
		n, ok := num(0, MaxPromoUses)
		if !ok {
			return again("aW:"+st.query, fmt.Sprintf(sw.badNum, 0, MaxPromoUses))
		}
		b.adminDraw(chat, 0, "aW:"+st.query+":"+strconv.FormatInt(n, 10), "")
	case "promo_c":
		code := cleanCode(text)
		if code == "" {
			return again("aW:"+st.query, fmt.Sprintf(sw.badCode, MaxPromoCode))
		}
		b.adminDraw(chat, 0, "aW:"+st.query+":"+code, "")
	case "tariff_rub":
		kop, ok := parseRub(text)
		if !ok || kop > MaxPriceRub*100 {
			return again("aF:"+strconv.FormatInt(st.user, 10), fmt.Sprintf(sw.badNum, 0, MaxPriceRub))
		}
		b.adminDraw(chat, 0, fmt.Sprintf("aF:%d:P:%d", st.user, kop), "")
	case "tariff_stars":
		n, ok := num(0, MaxPriceStars)
		if !ok {
			return again("aF:"+strconv.FormatInt(st.user, 10), fmt.Sprintf(sw.badNum, 0, MaxPriceStars))
		}
		b.adminDraw(chat, 0, fmt.Sprintf("aF:%d:X:%d", st.user, n), "")
	default:
		return false
	}
	return true
}

// parseRub reads rubles with up to two digits of kopecks («299», «299.9», «299,90») as kopecks.
func parseRub(s string) (int64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "₽")), ",", ".")
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(frac) > 2 {
		return 0, false
	}
	r, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || r < 0 || r > MaxPriceRub {
		return 0, false
	}
	k := int64(0)
	if frac != "" {
		for len(frac) < 2 {
			frac += "0"
		}
		if k, err = strconv.ParseInt(frac, 10, 64); err != nil || k < 0 {
			return 0, false
		}
	}
	return r*100 + k, true
}
