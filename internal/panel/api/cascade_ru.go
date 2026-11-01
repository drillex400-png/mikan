package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"mikan/internal/panel/geosync"
	"mikan/internal/panel/store/db"
)

// "Russia direct" of a cascade: of what leaves a node through another one, the traffic to
// Russian sites and addresses leaves the node itself, and only the rest goes on to the
// exit. It is a switch per inbound, and one for the node's relay (what other nodes send
// through it and it sends on). It is off until the admin turns it on.

// RUDirectView is the mode's state for a node.
type RUDirectView struct {
	Inbounds []string `json:"inbounds" doc:"Подключения этой ноды, у которых включён режим «РФ напрямую»"`
	Relay    bool     `json:"relay" doc:"Включён ли режим у служебного входа (трафик других нод, идущий дальше через эту)"`
	Data     *RUData  `json:"geodata,omitempty" doc:"Базы GeoIP и GeoSite, из которых нода узнаёт российские адреса"`
}

// RUData is whether the panel holds the databases the mode needs.
type RUData struct {
	Have      bool   `json:"have"`
	FetchedAt string `json:"fetched_at,omitempty"`
	Error     string `json:"error,omitempty" doc:"Почему базы не скачались; режим не включается на ноде, пока их нет"`
}

type ruDirectOutput struct{ Body RUDirectView }

type ruDirectPutInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Inbound string `json:"inbound,omitempty" maxLength:"128" doc:"Имя подключения ноды; пусто: служебный вход"`
		Relay   bool   `json:"relay,omitempty" doc:"Переключить служебный вход вместо подключения"`
		On      bool   `json:"on"`
	}
}

func (h *handlers) registerRUDirect() {
	tags := []string{"node"}
	huma.Register(h.api, huma.Operation{OperationID: "get-node-ru-direct", Method: http.MethodGet, Path: "/api/v1/nodes/{id}/cascade/ru-direct", Summary: "Каскад: режим «РФ напрямую»", Tags: tags}, h.getRUDirect)
	huma.Register(h.api, huma.Operation{OperationID: "set-node-ru-direct", Metadata: sessionOnly, Extensions: sessionOnlyExt, Method: http.MethodPut, Path: "/api/v1/nodes/{id}/cascade/ru-direct", Summary: "Каскад: включить или выключить «РФ напрямую»", Tags: tags}, h.setRUDirect)
	huma.Register(h.api, huma.Operation{OperationID: "refresh-ru-geodata", Metadata: sessionOnly, Extensions: sessionOnlyExt, Method: http.MethodPost, Path: "/api/v1/cascade/ru-direct/geodata", Summary: "Скачать базы для «РФ напрямую» сейчас", Tags: tags}, h.refreshRUGeodata)
}

func (h *handlers) ruDirectView(ctx context.Context, id int64) (RUDirectView, error) {
	q := h.d.Store.Q
	v := RUDirectView{Inbounds: []string{}}
	ins, err := q.ListInbounds(ctx)
	if err != nil {
		return v, err
	}
	on, err := q.ListRUDirectInbounds(ctx)
	if err != nil {
		return v, err
	}
	set := map[int64]bool{}
	for _, x := range on {
		if x.Valid {
			set[x.Int64] = true
		}
	}
	for _, in := range ins {
		if in.NodeID == id && set[in.ID] {
			v.Inbounds = append(v.Inbounds, in.Name)
		}
	}
	relays, err := q.ListRUDirectRelays(ctx)
	if err != nil {
		return v, err
	}
	for _, x := range relays {
		if x.Valid && x.Int64 == id {
			v.Relay = true
		}
	}
	if h.d.GeoData != nil {
		st := h.d.GeoData.Status()
		d := &RUData{Have: st.Have, Error: st.Error}
		if !st.FetchedAt.IsZero() {
			d.FetchedAt = st.FetchedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		v.Data = d
	}
	return v, nil
}

func (h *handlers) getRUDirect(ctx context.Context, in *nodeIDInput) (*ruDirectOutput, error) {
	if _, err := h.getNode(ctx, in.ID); err != nil {
		return nil, err
	}
	v, err := h.ruDirectView(ctx, in.ID)
	return &ruDirectOutput{Body: v}, err
}

func (h *handlers) setRUDirect(ctx context.Context, in *ruDirectPutInput) (*ruDirectOutput, error) {
	if _, err := h.getNode(ctx, in.ID); err != nil {
		return nil, err
	}
	b := in.Body
	if b.Relay == (b.Inbound != "") {
		return nil, fieldErr("body.inbound", "ru_direct_one_target")
	}
	target := "relay"
	err := h.d.Store.Tx(ctx, func(q *db.Queries) error {
		if b.Relay {
			r, err := q.GetNodeRelay(ctx, in.ID)
			if errors.Is(err, sql.ErrNoRows) || err == nil && !r.ExitNodeID.Valid {
				// Nothing to split: the relay's traffic does not go on to another node.
				return errNoExit
			}
			if err != nil {
				return err
			}
			if b.On {
				return q.SetRelayRUDirect(ctx, sql.NullInt64{Int64: in.ID, Valid: true})
			}
			return q.ClearRelayRUDirect(ctx, sql.NullInt64{Int64: in.ID, Valid: true})
		}
		target = b.Inbound
		ins, err := q.ListNodeInbounds(ctx, in.ID)
		if err != nil {
			return err
		}
		for _, x := range ins {
			if x.Name != b.Inbound {
				continue
			}
			if !x.ExitNodeID.Valid {
				return errNoExit
			}
			if b.On {
				return q.SetInboundRUDirect(ctx, sql.NullInt64{Int64: x.ID, Valid: true})
			}
			return q.ClearInboundRUDirect(ctx, sql.NullInt64{Int64: x.ID, Valid: true})
		}
		return sql.ErrNoRows
	})
	switch {
	case errors.Is(err, errNoExit):
		return nil, fieldErr("body.inbound", "ru_direct_no_exit")
	case errors.Is(err, sql.ErrNoRows):
		return nil, huma.Error404NotFound("not_found")
	case err != nil:
		return nil, err
	}
	// The databases are fetched in the background once somebody asks for the mode: the
	// node gets them with its next state, the switch does not wait for a download.
	if b.On && h.d.GeoData != nil && !h.d.GeoData.Status().Have {
		go func() { _ = h.d.GeoData.Refresh(context.WithoutCancel(ctx)) }()
	}
	h.d.Changes.SlotsChanged()
	h.audit(ctx, sessionOf(ctx).AdminID, "node.cascade_ru_direct", "node", strconv.FormatInt(in.ID, 10), map[string]any{"target": target, "on": b.On})
	v, err := h.ruDirectView(ctx, in.ID)
	return &ruDirectOutput{Body: v}, err
}

var errNoExit = errors.New("no_exit")

type ruRefreshOutput struct{ Body RUData }

func (h *handlers) refreshRUGeodata(ctx context.Context, _ *struct{}) (*ruRefreshOutput, error) {
	if h.d.GeoData == nil {
		return nil, huma.Error404NotFound("not_found")
	}
	if err := h.d.GeoData.Refresh(ctx); err != nil {
		return &ruRefreshOutput{Body: RUData{Error: err.Error()}}, nil
	}
	st := h.d.GeoData.Status()
	h.d.Changes.SlotsChanged()
	h.audit(ctx, sessionOf(ctx).AdminID, "cascade.geodata_refresh", "geodata", "", nil)
	return &ruRefreshOutput{Body: RUData{Have: st.Have, FetchedAt: st.FetchedAt.UTC().Format("2006-01-02T15:04:05Z")}}, nil
}

var _ = geosync.Every
