package tgbot

import (
	"context"
	"errors"
	"net"

	"mikan/internal/panel/model"
)

// Why an admin's screen failed, in the words the admin needs: the cause, not the call. The
// raw error stays in the log; the screen shows one of these.
const (
	failOther  = iota // anything not told apart below
	failDown          // the panel did not answer
	failOld           // the panel does not know the call (an older panel than the bot)
	failInput         // the panel refused what was asked
	failBusy          // a broadcast is already being sent
	failBotOff        // the bot is off, a broadcast has nobody to reach
)

// failKind tells why err happened.
func failKind(err error) int {
	var rpc *RPCError
	var ne net.Error
	switch {
	case errors.Is(err, model.ErrAdminInput):
		return failInput
	case errors.Is(err, model.ErrBroadcastBusy):
		return failBusy
	case errors.Is(err, model.ErrBroadcastOff):
		return failBotOff
	case errors.As(err, &rpc) && rpc.Code == "no_method":
		return failOld
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.As(err, &ne):
		return failDown
	}
	return failOther
}

// failText is the sentence for err.
func failText(aw *admWords, err error) string {
	switch failKind(err) {
	case failDown:
		return aw.unavailable
	case failOld:
		return aw.failOld
	case failInput:
		return aw.failInput
	case failBusy:
		return aw.bcBusy
	case failBotOff:
		return aw.bcOff
	}
	return aw.failed
}
