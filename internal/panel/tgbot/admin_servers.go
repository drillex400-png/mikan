package tgbot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mikan/internal/panel/model"
	"mikan/internal/textutil"
	"mikan/internal/tgapi"
)

// The admin's servers: the list with a state for each, a card for the one chosen, and what the
// panel does with a node from the admin panel. A card carries no node key, certificate or API
// address. The buttons are "aS:<id>[:<action>[:<argument>]]" (and "an" for the list):
//
//	aS:<id>        the card              aS:<id>:c   check now
//	aS:<id>:e      turn on or off (asks) aS:<id>:s   speed test
//	aS:<id>:u      update (asks)         aS:<id>:r   restart containers (asks)
//	aS:<id>:d      remove (asks twice)   aS:<id>:n|p|m   ask for the name, public name or channel
//	aS:<id>:z:<k>  the confirmed one: k is e1, e0, u, r, d

// Words of the servers' screens.
type srvWords struct {
	list, listHint, card, status, online, offline, disabled, unknown, local          string
	version, behind, upToDate, load, cpu, mem, conns, traffic, listeners, inbounds   string
	channel, noChannel, publicName, noPublic, domain, host, since, lastError         string
	turnOn, turnOff, check, speed, update, restart, remove, rename, rePublic, reChan string
	askOn, askOff, askUpdate, askRestart, askRemove, askRemove2                      string
	askName, askPublic, askChan, badName, badChan                                    string
	checked, changed, updating, updateStarted, restarted, removed                    string
	speedTitle, ping, loss, down, up, speedFail                                      string
	eNodeLocal, eNodeOffline, eNotBehind, eUpdating, eCannot, eFailed, eInUse        string
	updateRunning, back                                                              string
}

var srvRU = srvWords{
	list: "Серверы", listHint: "Выберите сервер.", card: "Сервер", status: "Состояние", online: "работает", offline: "нет связи",
	disabled: "выключен", unknown: "ещё не проверен", local: "основной сервер панели",
	version: "Версия", behind: "старее панели", upToDate: "актуальная", load: "Нагрузка", cpu: "процессор", mem: "память",
	conns: "Подключения", traffic: "Трафик за сутки", listeners: "Протоколы", inbounds: "Подключений настроено",
	channel: "Канал", noChannel: "не задан", publicName: "Публичное имя", noPublic: "скрыт", domain: "Домен", host: "Адрес",
	since: "нет связи с", lastError: "Причина",
	turnOn: "🟢 Включить", turnOff: "⏸ Выключить", check: "🔄 Проверить", speed: "⚡ Скорость", update: "⬆️ Обновить",
	restart: "♻️ Перезапуск", remove: "🗑 Удалить", rename: "✏️ Имя", rePublic: "🏷 Публичное имя", reChan: "📶 Канал",
	askOn:      "Включить сервер «%s»? Клиенты снова получат его в подписке.",
	askOff:     "Выключить сервер «%s»? Клиенты перестанут получать его в подписке.",
	askUpdate:  "Обновить сервер «%s» до версии панели? Он ненадолго перезапустится.",
	askRestart: "Перезапустить контейнеры сервера «%s»? Подключения клиентов на нём прервутся на несколько секунд.",
	askRemove:  "Удалить сервер «%s»? Это необратимо.",
	askRemove2: "Точно удалить «%s»? Его подключения исчезнут из подписок, а выходы каскада уйдут на другие серверы.",
	askName:    "Пришлите новое имя сервера (до %d символов).",
	askPublic:  "Пришлите публичное имя сервера для канала состояния (до %d символов) или «-», чтобы скрыть его.",
	askChan:    "Пришлите ширину канала в Мбит/с (1–100000) в каждую сторону.",
	badName:    "Имя не подошло: оно пустое, длиннее %d символов, занято или совпадает с названием группы.",
	badChan:    "Нужно число от 1 до 100000.",
	checked:    "Проверено.", changed: "Готово.", updating: "обновляется…", updateStarted: "Обновление запущено. Сервер перезапустится сам.",
	restarted: "Перезапуск запущен.", removed: "Сервер удалён.",
	speedTitle: "Скорость сервера", ping: "Задержка", loss: "Потери", down: "Загрузка", up: "Отдача", speedFail: "Тест не удался",
	eNodeLocal:    "Это основной сервер панели, для него действие недоступно.",
	eNodeOffline:  "Сервер не отвечает. Проверьте его и попробуйте снова.",
	eNotBehind:    "Сервер уже на версии панели.",
	eUpdating:     "Обновление этого сервера уже идёт.",
	eCannot:       "Этот сервер не умеет обновляться сам: его нужно один раз обновить вручную.",
	eFailed:       "Сервер не выполнил действие. Попробуйте позже.",
	eInUse:        "Сервер нужен: через него бот выходит в Telegram, либо у тарифа не останется серверов. Сначала смените это в панели.",
	updateRunning: "идёт обновление", back: "← Серверы",
}

var srvEN = srvWords{
	list: "Servers", listHint: "Pick a server.", card: "Server", status: "State", online: "up", offline: "unreachable",
	disabled: "off", unknown: "not checked yet", local: "the panel's own server",
	version: "Version", behind: "older than the panel", upToDate: "current", load: "Load", cpu: "CPU", mem: "memory",
	conns: "Connections", traffic: "Traffic, 24 h", listeners: "Protocols", inbounds: "Connections set up",
	channel: "Channel", noChannel: "not set", publicName: "Public name", noPublic: "hidden", domain: "Domain", host: "Address",
	since: "unreachable since", lastError: "Reason",
	turnOn: "🟢 Turn on", turnOff: "⏸ Turn off", check: "🔄 Check", speed: "⚡ Speed", update: "⬆️ Update",
	restart: "♻️ Restart", remove: "🗑 Remove", rename: "✏️ Name", rePublic: "🏷 Public name", reChan: "📶 Channel",
	askOn:      "Turn on the server “%s”? Clients get it in their subscription again.",
	askOff:     "Turn off the server “%s”? Clients stop getting it in their subscription.",
	askUpdate:  "Update the server “%s” to the panel's version? It restarts briefly.",
	askRestart: "Restart the containers of the server “%s”? Clients on it are cut off for a few seconds.",
	askRemove:  "Remove the server “%s”? This cannot be undone.",
	askRemove2: "Really remove “%s”? Its connections leave the subscriptions and cascade exits move to other servers.",
	askName:    "Send the new name of the server (up to %d characters).",
	askPublic:  "Send the public name for the status channel (up to %d characters), or “-” to hide the server.",
	askChan:    "Send the channel width in Mbit/s (1–100000) each way.",
	badName:    "That name does not fit: it is empty, longer than %d characters, taken, or the name of a group.",
	badChan:    "A number from 1 to 100000 is needed.",
	checked:    "Checked.", changed: "Done.", updating: "updating…", updateStarted: "The update has started. The server restarts by itself.",
	restarted: "The restart has started.", removed: "The server is removed.",
	speedTitle: "Server speed", ping: "Ping", loss: "Loss", down: "Download", up: "Upload", speedFail: "The test failed",
	eNodeLocal:    "This is the panel's own server; the action is not available for it.",
	eNodeOffline:  "The server does not answer. Check it and try again.",
	eNotBehind:    "The server is on the panel's version already.",
	eUpdating:     "An update of this server is already running.",
	eCannot:       "This server cannot update itself: it needs a manual update once.",
	eFailed:       "The server could not do it. Try again later.",
	eInUse:        "The server is needed: the bot reaches Telegram through it, or a tariff would be left with no servers. Change that in the panel first.",
	updateRunning: "update running", back: "← Servers",
}

func srvWordsFor(lang string) *srvWords { return textutil.ByLang(lang, &srvRU, &srvEN) }

// srvFail says why a server action failed, in the words the admin needs; ok false: not one of
// the servers' own errors.
func srvFail(sw *srvWords, err error) (string, bool) {
	switch {
	case errors.Is(err, model.ErrNodeLocal):
		return sw.eNodeLocal, true
	case errors.Is(err, model.ErrNodeOffline):
		return sw.eNodeOffline, true
	case errors.Is(err, model.ErrNodeNotBehind):
		return sw.eNotBehind, true
	case errors.Is(err, model.ErrNodeUpdating):
		return sw.eUpdating, true
	case errors.Is(err, model.ErrNodeCannot):
		return sw.eCannot, true
	case errors.Is(err, model.ErrNodeInUse):
		return sw.eInUse, true
	case errors.Is(err, model.ErrNodeFailed):
		return sw.eFailed, true
	}
	return "", false
}

// nodeDot: the colour of a server's state.
func nodeDot(n AdminNode) string {
	switch {
	case !n.Enabled:
		return "⚫"
	case n.Online:
		return "🟢"
	}
	return "🔴"
}

func nodeState(sw *srvWords, n AdminNode) string {
	switch {
	case !n.Enabled:
		return sw.disabled
	case n.Online:
		return sw.online
	}
	return sw.offline
}

// pct prints a percentage the way the card shows it.
func pct(f float64) string { return strconv.FormatFloat(f, 'f', 0, 64) + "%" }

// sizeOf prints bytes in the unit that reads best.
func sizeOf(b uint64) string {
	const k = 1024.0
	f := float64(b)
	switch {
	case f >= k*k*k:
		return strconv.FormatFloat(f/(k*k*k), 'f', 1, 64) + " GB"
	case f >= k*k:
		return strconv.FormatFloat(f/(k*k), 'f', 0, 64) + " MB"
	}
	return strconv.FormatFloat(f/k, 'f', 0, 64) + " KB"
}

func mbps(bps int64) string { return strconv.FormatFloat(float64(bps)/1e6, 'f', 1, 64) + " Mbit/s" }

// srvList draws the list: one button a server, with its state.
func (b *Bot) srvList(ctx context.Context, adm Admin, chat int64, aw *admWords, sw *srvWords, pre string,
	fail func(error) (string, *tgapi.Keyboard), home []tgapi.Button) (string, *tgapi.Keyboard) {
	ns, err := adm.Nodes(ctx, chat)
	if err != nil {
		return fail(err)
	}
	var rowsText []Row
	var rows [][]tgapi.Button
	for _, n := range ns {
		v := nodeState(sw, n)
		if n.Version != "" && n.Online {
			v += " · " + n.Version
			if n.Behind {
				v += " ⬆"
			}
		}
		rowsText = append(rowsText, Row{Icon: nodeDot(n), Label: n.Name, Value: v})
		rows = append(rows, []tgapi.Button{{Text: truncate(nodeDot(n)+" "+n.Name, 40), CallbackData: "aS:" + strconv.FormatInt(n.ID, 10)}})
	}
	body := block("", rowsText...)
	if len(ns) == 0 {
		body = html.EscapeString(aw.noNodes)
	} else {
		body += "\n\n<i>" + html.EscapeString(sw.listHint) + "</i>"
	}
	lines := []string{head("🖥", sw.list), "", body}
	rows = append(rows, []tgapi.Button{{Text: aw.refresh, CallbackData: "an"}}, home)
	return pre + strings.Join(lines, "\n"), &tgapi.Keyboard{InlineKeyboard: rows}
}

// srvCard draws one server and the buttons of what can be done with it.
func (b *Bot) srvCard(ctx context.Context, adm Admin, chat, id int64, aw *admWords, sw *srvWords, pre string,
	fail func(error) (string, *tgapi.Keyboard)) (string, *tgapi.Keyboard) {
	n, err := adm.Node(ctx, chat, id)
	if err != nil {
		return fail(err)
	}
	return b.srvCardOf(n, aw, sw, pre)
}

func (b *Bot) srvCardOf(n AdminNodeInfo, aw *admWords, sw *srvWords, pre string) (string, *tgapi.Keyboard) {
	ids := strconv.FormatInt(n.ID, 10)
	state := nodeState(sw, n.AdminNode)
	if n.Local {
		state += " · " + sw.local
	}
	if n.Updating {
		state += " · " + sw.updateRunning
	}
	status := []Row{{"", sw.status, state}}
	if n.Status == "error" && n.Enabled {
		if n.Error != "" {
			status = append(status, Row{"⚠️", sw.lastError, truncate(n.Error, 120)})
		}
		if n.ErrorSince > 0 {
			status = append(status, Row{"🕒", sw.since, time.Unix(n.ErrorSince, 0).Format("02.01 15:04")})
		}
	}
	if n.Host != "" {
		status = append(status, Row{"🌐", sw.host, n.Host})
	}
	if n.Domain != "" {
		status = append(status, Row{"🔗", sw.domain, n.Domain})
	}
	parts := []string{head(nodeDot(n.AdminNode), n.Name), block("", status...)}
	if n.Online {
		ver := n.Version
		if n.Behind {
			ver += " (" + sw.behind + ")"
		} else if n.Version != "" {
			ver += " (" + sw.upToDate + ")"
		}
		live := []Row{{"🏷", sw.version, ver}}
		if n.Listeners > 0 {
			live = append(live, Row{"🔌", sw.listeners, fmt.Sprintf("%d/%d", n.ListenersOK, n.Listeners)})
		}
		live = append(live, Row{"👥", sw.conns, strconv.Itoa(n.Conns)})
		blockLive := block("", live...)
		if n.MemTotal > 0 {
			blockLive = "<blockquote>" + trimQuote(blockLive) + "\n" +
				gauge(sw.cpu, int64(n.CPU), 100) + "\n" +
				gauge(sw.mem, int64(n.MemUsed>>20), int64(n.MemTotal>>20)) + "\n" +
				html.EscapeString(sizeOf(n.MemUsed)+" / "+sizeOf(n.MemTotal)) + "</blockquote>"
		}
		parts = append(parts, blockLive)
	}
	ch := sw.noChannel
	if n.ChannelMbps > 0 {
		ch = strconv.FormatInt(n.ChannelMbps, 10) + " Mbit/s"
	}
	pub := sw.noPublic
	if n.PublicName != "" {
		pub = n.PublicName
	}
	parts = append(parts, block("",
		Row{"🧩", sw.inbounds, strconv.Itoa(n.Inbounds)},
		Row{"📊", sw.traffic, sizeOf(uint64(maxInt64(n.Traffic24h, 0)))},
		Row{"⚡", sw.channel, ch},
		Row{"🏷", sw.publicName, pub}))

	btn := func(text, data string) tgapi.Button {
		return tgapi.Button{Text: text, CallbackData: "aS:" + ids + data}
	}
	toggle := clr(tgapi.StyleDanger, btn(sw.turnOff, ":e"))
	if !n.Enabled {
		toggle = clr(tgapi.StyleSuccess, btn(sw.turnOn, ":e"))
	}
	rows := [][]tgapi.Button{{toggle, btn(sw.check, ":c")}}
	if !n.Local {
		row := []tgapi.Button{btn(sw.speed, ":s")}
		if n.Behind && n.CanUpdate && !n.Updating {
			row = append(row, clr(tgapi.StylePrimary, btn(sw.update, ":u")))
		}
		rows = append(rows, row, []tgapi.Button{clr(tgapi.StyleDanger, btn(sw.restart, ":r")), clr(tgapi.StyleDanger, btn(sw.remove, ":d"))})
	} else {
		rows = append(rows, []tgapi.Button{btn(sw.speed, ":s")})
	}
	rows = append(rows, []tgapi.Button{btn(sw.rename, ":n"), btn(sw.rePublic, ":p"), btn(sw.reChan, ":m")},
		[]tgapi.Button{{Text: sw.back, CallbackData: "an"}, {Text: aw.back, CallbackData: "a"}})
	return pre + join(parts...), &tgapi.Keyboard{InlineKeyboard: rows}
}

// trimQuote takes the quote tags off a block, to add lines to it.
func trimQuote(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, "<blockquote>"), "</blockquote>")
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// srvScreen answers "aS:<id>[:<action>[:<argument>]]".
func (b *Bot) srvScreen(ctx context.Context, adm Admin, chat int64, arg string, aw *admWords, sw *srvWords, pre string,
	fail func(error) (string, *tgapi.Keyboard), home []tgapi.Button) (string, *tgapi.Keyboard) {
	idS, rest, _ := strings.Cut(arg, ":")
	id := atoi(idS)
	if id <= 0 {
		return fail(model.ErrNotFound)
	}
	act, val, _ := strings.Cut(rest, ":")
	kb := func(rows ...[]tgapi.Button) *tgapi.Keyboard { return &tgapi.Keyboard{InlineKeyboard: rows} }
	back := []tgapi.Button{{Text: sw.back, CallbackData: "aS:" + idS}}
	// srvErr: a server's own refusal in its words, any other as the screens say it.
	srvErr := func(err error) (string, *tgapi.Keyboard) {
		if s, ok := srvFail(sw, err); ok {
			return pre + html.EscapeString(s), kb(back, home)
		}
		return fail(err)
	}
	card := func(notice string) (string, *tgapi.Keyboard) {
		return b.srvCard(ctx, adm, chat, id, aw, sw, html.EscapeString(notice)+"\n\n", fail)
	}
	nameOf := func() (string, error) {
		n, err := adm.Node(ctx, chat, id)
		return n.Name, err
	}
	ask := func(format string) (string, *tgapi.Keyboard) {
		name, err := nameOf()
		if err != nil {
			return srvErr(err)
		}
		return pre + html.EscapeString(fmt.Sprintf(format, name)), confirm(aw, "aS:"+idS+":z:"+act+val, "aS:"+idS)
	}

	switch act {
	case "":
		return b.srvCard(ctx, adm, chat, id, aw, sw, pre, fail)
	case "c":
		n, err := adm.CheckNode(ctx, chat, id)
		if err != nil {
			return srvErr(err)
		}
		return b.srvCardOf(n, aw, sw, html.EscapeString(sw.checked)+"\n\n")
	case "s":
		sp, err := adm.SpeedNode(ctx, chat, id)
		if err != nil {
			return srvErr(err)
		}
		lines := []string{head("⚡", sw.speedTitle), ""}
		if sp.Error != "" {
			lines = append(lines, html.EscapeString(sw.speedFail+": "+truncate(sp.Error, 120)))
		} else {
			lines = append(lines, kv(sw.ping, strconv.FormatFloat(sp.PingMs, 'f', 0, 64)+" ms"), kv(sw.loss, strconv.FormatFloat(sp.LossPct, 'f', 1, 64)+"%"),
				kv(sw.down, mbps(sp.DownBps)), kv(sw.up, mbps(sp.UpBps)))
		}
		return pre + strings.Join(lines, "\n"), kb(back, home)
	case "e": // asks; z:e1 / z:e0 does it
		n, err := adm.Node(ctx, chat, id)
		if err != nil {
			return srvErr(err)
		}
		if n.Enabled {
			return pre + html.EscapeString(fmt.Sprintf(sw.askOff, n.Name)), confirm(aw, "aS:"+idS+":z:e0", "aS:"+idS)
		}
		return pre + html.EscapeString(fmt.Sprintf(sw.askOn, n.Name)), confirm(aw, "aS:"+idS+":z:e1", "aS:"+idS)
	case "u":
		return ask(sw.askUpdate)
	case "r":
		return ask(sw.askRestart)
	case "d": // the first ask; "d" with the argument 2 is the second, and z:d the removal
		name, err := nameOf()
		if err != nil {
			return srvErr(err)
		}
		if val == "2" {
			return pre + html.EscapeString(fmt.Sprintf(sw.askRemove2, name)), confirm(aw, "aS:"+idS+":z:d", "aS:"+idS)
		}
		return pre + html.EscapeString(fmt.Sprintf(sw.askRemove, name)), confirm(aw, "aS:"+idS+":d:2", "aS:"+idS)
	case "n", "p", "m":
		kind, text := "nname", fmt.Sprintf(sw.askName, MaxNodeText)
		switch act {
		case "p":
			kind, text = "npublic", fmt.Sprintf(sw.askPublic, MaxNodeText)
		case "m":
			kind, text = "nchannel", sw.askChan
		}
		b.setWait(chat, adminState{kind: kind, user: id})
		return pre + html.EscapeString(text), kb([]tgapi.Button{{Text: aw.cancel, CallbackData: "aS:" + idS}})
	case "z": // the confirmed one
		var err error
		note := sw.changed
		switch val {
		case "e1", "e0":
			_, err = adm.SetNodeEnabled(ctx, chat, id, val == "e1")
		case "u":
			_, err = adm.UpdateNode(ctx, chat, id)
			note = sw.updateStarted
		case "r":
			err = adm.RestartNode(ctx, chat, id)
			note = sw.restarted
		case "d":
			if err = adm.DeleteNode(ctx, chat, id); err == nil {
				return html.EscapeString(sw.removed), kb([]tgapi.Button{{Text: sw.back, CallbackData: "an"}}, home)
			}
		default:
			return fail(model.ErrAdminInput)
		}
		if err != nil {
			return srvErr(err)
		}
		return card(note)
	}
	return fail(model.ErrAdminInput)
}

// srvInput checks the text the admin sent for a server's name, public name or channel and
// says which field it is for; notice is what to tell when it does not fit (the bot then waits
// again). ok false: the state is not a server's.
func srvInput(st adminState, text string, sw *srvWords) (field, value, notice string, ok bool) {
	switch st.kind {
	case "nname":
		if n := utf8.RuneCountInString(text); n == 0 || n > MaxNodeText {
			return "", "", fmt.Sprintf(sw.badName, MaxNodeText), true
		}
		return NodeFieldName, text, "", true
	case "npublic":
		if text == "-" {
			text = ""
		}
		if utf8.RuneCountInString(text) > MaxNodeText {
			return "", "", fmt.Sprintf(sw.badName, MaxNodeText), true
		}
		return NodeFieldPublic, text, "", true
	case "nchannel":
		if n, err := strconv.ParseInt(text, 10, 64); err != nil || n < 1 || n > 100000 {
			return "", "", sw.badChan, true
		}
		return NodeFieldChannel, text, "", true
	}
	return "", "", "", false
}

// srvText handles the admin's next message when the bot waited for a server's name, public
// name or channel. It reports whether it was one of those.
func (b *Bot) srvText(chat int64, st adminState, text string, aw *admWords, sw *srvWords) bool {
	field, value, notice, ok := srvInput(st, text, sw)
	if !ok {
		return false
	}
	if notice != "" {
		b.setWait(chat, st)
		b.adminDraw(chat, 0, "aS:"+strconv.FormatInt(st.user, 10), notice)
		return true
	}
	b.d.Outbox.Reply(chat, "adm:"+strconv.FormatInt(chat, 10), 1, func(ctx context.Context, c *tgapi.Client) error {
		adm, ok := b.adminOf(ctx, chat)
		if !ok {
			return nil
		}
		n, err := adm.SetNodeField(ctx, chat, st.user, field, value)
		var out string
		var kb *tgapi.Keyboard
		switch {
		case errors.Is(err, model.ErrAdminInput):
			// The panel's rules refused it (a name that is taken, a group's name): ask again.
			b.setWait(chat, st)
			out, kb = b.srvCard(ctx, adm, chat, st.user, aw, sw, html.EscapeString(fmt.Sprintf(sw.badName, MaxNodeText))+"\n\n", func(error) (string, *tgapi.Keyboard) {
				return html.EscapeString(aw.failed), nil
			})
		case err != nil:
			if s, ok := srvFail(sw, err); ok {
				out = html.EscapeString(s)
			} else {
				out = html.EscapeString(failText(aw, err))
			}
		default:
			out, kb = b.srvCardOf(n, aw, sw, html.EscapeString(sw.changed)+"\n\n")
		}
		_, err = c.Send(ctx, chat, out, kb, false)
		return err
	})
	return true
}
