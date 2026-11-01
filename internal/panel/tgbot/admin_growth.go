package tgbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"mikan/internal/panel/growth"
	"mikan/internal/tgapi"
)

// The admin's screen of the referral programme (callback aG). Rewards are picked from buttons,
// so no text is waited for. A change is written to the audit log first and saved after, and the
// clients' command menu follows the switch at once.

type growthAdminWords struct {
	btn, title, state, on, off, inviter, friend, days, turnOn, turnOff, paid, hint, failed string
}

var growthAdminRU = growthAdminWords{
	btn: "🎁 Рост", title: "Реферальная программа", state: "Состояние", on: "включена", off: "выключена",
	inviter: "Пригласившему", friend: "Приглашённому", days: "дн.", turnOn: "Включить", turnOff: "Выключить",
	paid: "Оплативших друзей", hint: "Награда выдаётся один раз, за первую оплату друга.",
	failed: "Не удалось сохранить настройки. Попробуйте ещё раз.",
}

var growthAdminEN = growthAdminWords{
	btn: "🎁 Growth", title: "Referral programme", state: "State", on: "on", off: "off",
	inviter: "Inviter gets", friend: "Friend gets", days: "d", turnOn: "Turn on", turnOff: "Turn off",
	paid: "Friends who paid", hint: "The reward is given once, for the friend's first payment.",
	failed: "Could not save the settings. Try again.",
}

func growthAdminWordsFor(lang string) *growthAdminWords {
	if lang == "en" {
		return &growthAdminEN
	}
	return &growthAdminRU
}

// rewardChoices are the values offered for a reward in days; 0 gives nothing to that side.
var rewardChoices = []int64{0, 3, 7, 14, 30}

// growthScreen draws the screen, first making the change a callback asks for.
func (b *Bot) growthScreen(ctx context.Context, adm Admin, chat int64, arg string, home []tgapi.Button) (string, *tgapi.Keyboard) {
	cfg := b.config(ctx)
	gw := growthAdminWordsFor(cfg.Lang)
	g := b.growth()
	if g == nil {
		return head("🎁", gw.title) + "\n" + gw.failed, &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
	}
	st, err := g.Settings(ctx)
	if err != nil {
		return head("🎁", gw.title) + "\n" + gw.failed, &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
	}
	if next, changed := applyGrowthChange(st, arg); changed {
		if err := adm.NoteGrowth(ctx, chat, next.Enabled, next.InviterDays, next.FriendDays); err != nil {
			return head("🎁", gw.title) + "\n" + gw.failed, &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
		}
		if err := g.SaveSettings(ctx, next); err != nil {
			b.d.Log.Warn("growth: save", "err", err)
			return head("🎁", gw.title) + "\n" + gw.failed, &tgapi.Keyboard{InlineKeyboard: [][]tgapi.Button{home}}
		}
		st = next
		b.d.Go(func() { b.refreshCommands(ctx) })
	}
	state := gw.off
	if st.Enabled {
		state = gw.on
	}
	text := head("🎁", gw.title) + "\n" +
		block("", Row{Icon: "▫️", Label: gw.state, Value: state},
			Row{Icon: "▫️", Label: gw.inviter, Value: dayWord(gw, st.InviterDays)},
			Row{Icon: "▫️", Label: gw.friend, Value: dayWord(gw, st.FriendDays)}) +
		"\n" + gw.hint
	rows := [][]tgapi.Button{{toggleButton(gw, st.Enabled)}}
	rows = append(rows, rewardRow("i", gw.inviter, st.InviterDays), rewardRow("f", gw.friend, st.FriendDays))
	rows = append(rows, home)
	return text, &tgapi.Keyboard{InlineKeyboard: rows}
}

func dayWord(gw *growthAdminWords, n int64) string { return fmt.Sprintf("%d %s", n, gw.days) }

func toggleButton(gw *growthAdminWords, on bool) tgapi.Button {
	if on {
		return clr(tgapi.StyleDanger, tgapi.Button{Text: gw.turnOff, CallbackData: "aG:t"})
	}
	return clr(tgapi.StyleSuccess, tgapi.Button{Text: gw.turnOn, CallbackData: "aG:t"})
}

// rewardRow is a row of buttons for one side's reward; the chosen one is marked.
func rewardRow(side, label string, cur int64) []tgapi.Button {
	row := make([]tgapi.Button, 0, len(rewardChoices))
	for _, d := range rewardChoices {
		t := strconv.FormatInt(d, 10)
		if d == cur {
			t = "✓ " + t
		}
		row = append(row, tgapi.Button{Text: t, CallbackData: "aG:" + side + ":" + strconv.FormatInt(d, 10)})
	}
	return row
}

// applyGrowthChange reads a callback argument (t, i:N or f:N) against the current settings.
// An unknown or out-of-range argument changes nothing, so a forged callback does no harm.
func applyGrowthChange(st growth.Settings, arg string) (growth.Settings, bool) {
	parts := strings.Split(arg, ":")
	switch {
	case arg == "t":
		st.Enabled = !st.Enabled
		return st, true
	case len(parts) == 2 && (parts[0] == "i" || parts[0] == "f"):
		n, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || !allowedReward(n) {
			return st, false
		}
		if parts[0] == "i" {
			if st.InviterDays == n {
				return st, false
			}
			st.InviterDays = n
		} else {
			if st.FriendDays == n {
				return st, false
			}
			st.FriendDays = n
		}
		return st, true
	}
	return st, false
}

func allowedReward(n int64) bool {
	for _, d := range rewardChoices {
		if d == n {
			return true
		}
	}
	return false
}

// refreshCommands sets the clients' command menu again, so /invite appears or goes with the
// switch and not at the bot's next start.
func (b *Bot) refreshCommands(ctx context.Context) {
	if b.d.Client == nil {
		return
	}
	cs, _ := b.commandMenu(ctx, false)
	if err := b.d.Client.SetCommandList(ctx, cs); err != nil {
		b.d.Log.Warn("growth: command menu", "err", err)
	}
}
