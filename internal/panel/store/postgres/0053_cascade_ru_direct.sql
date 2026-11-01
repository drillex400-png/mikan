-- +goose Up
-- "Russia direct" of a cascade: of what leaves through another node, the traffic to Russian
-- sites and addresses leaves the source node itself. One row per inbound or relay that has
-- the mode on; no row is off, as before it. A table of its own keeps inbounds and node_relays
-- as they are.
CREATE TABLE cascade_ru_direct (
  inbound_id BIGINT REFERENCES inbounds(id) ON DELETE CASCADE,
  relay_node_id BIGINT REFERENCES node_relays(node_id) ON DELETE CASCADE,
  CHECK ((inbound_id IS NOT NULL) <> (relay_node_id IS NOT NULL))
);
CREATE UNIQUE INDEX cascade_ru_direct_inbound ON cascade_ru_direct (inbound_id) WHERE inbound_id IS NOT NULL;
CREATE UNIQUE INDEX cascade_ru_direct_relay ON cascade_ru_direct (relay_node_id) WHERE relay_node_id IS NOT NULL;

-- +goose Down
DROP TABLE cascade_ru_direct;
