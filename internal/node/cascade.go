package node

import (
	"encoding/json"
	"fmt"
	"strings"

	"mikan/internal/nodeapi"
	"mikan/internal/proto"
)

// listenerNames are the listeners the state runs, the relay included: rules may name
// only these.
func listenerNames(st nodeapi.DesiredState) map[string]bool {
	present := map[string]bool{}
	for _, in := range st.Inbounds {
		present[in.Name] = true
	}
	if st.Relay != nil {
		present[nodeapi.RelayListener] = true
	}
	return present
}

// relayListener is the hidden VLESS REALITY door other nodes of the panel come in by.
func relayListener(r *nodeapi.Relay, cert proto.Cert) (map[string]any, error) {
	t, err := proto.FromJSON(r.Config)
	if err != nil {
		return nil, err
	}
	if t.Type() != "vless" || t["reality-config"] == nil {
		return nil, fmt.Errorf("the relay is VLESS REALITY")
	}
	return proto.Listener(t, nodeapi.RelayListener, "", r.Port, r.Users, cert, proto.Options{})
}

// exitProxyConfig is the outbound to another node's relay. The panel builds it; the node
// only names it and keeps it to the protocol relays speak.
func exitProxyConfig(e nodeapi.Exit) (map[string]any, error) {
	if !strings.HasPrefix(e.Name, "NODE-") || !safeRuleValue(e.Name) {
		return nil, fmt.Errorf("exit name %q", e.Name)
	}
	var p map[string]any
	if err := json.Unmarshal(e.Proxy, &p); err != nil {
		return nil, fmt.Errorf("exit %s: %w", e.Name, err)
	}
	if p["type"] != "vless" {
		return nil, fmt.Errorf("exit %s: the relay is VLESS", e.Name)
	}
	p["name"] = e.Name
	return p, nil
}

// exitRules send the inbounds of each exit to that node. An inbound named by several
// exits goes to the first.
//
// With geo (the node holds readable geodata, see Geo) an inbound of RUDirect first lets the
// Russian sites and addresses out of the node itself: the rules for it come before the one
// that sends it on, so the first match wins and the rest of its traffic goes to the exit.
// Without geo the inbound goes to the exit whole, as it did before the mode: a rule that
// names a database the node does not have would not load, and nothing else would apply.
func exitRules(st nodeapi.DesiredState, geo bool) []string {
	present := listenerNames(st)
	taken := map[string]bool{}
	var r []string
	for _, e := range st.Exits {
		if !strings.HasPrefix(e.Name, "NODE-") || !safeRuleValue(e.Name) {
			continue
		}
		local := map[string]bool{}
		for _, name := range e.RUDirect {
			local[name] = true
		}
		for _, name := range e.Inbounds {
			if !present[name] || taken[name] || !safeRuleValue(name) {
				continue
			}
			taken[name] = true
			if geo && local[name] {
				// no-resolve: the destination is judged by the address the client gave; a name
				// is judged by geosite, and the rest of the traffic goes on by name to the exit
				// (the exit resolves it, so the answer does not come from this node's country).
				r = append(r,
					"AND,((IN-NAME,"+name+"),(GEOSITE,category-ru)),DIRECT",
					"AND,((IN-NAME,"+name+"),(GEOIP,ru,no-resolve)),DIRECT")
			}
			r = append(r, "IN-NAME,"+name+","+e.Name)
		}
	}
	return r
}
