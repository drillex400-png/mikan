// "Russia direct" right under the exit node: of what leaves through another node, the
// Russian sites leave from this one. Once the exit is saved the switch is live (the same one
// as on the node's cascade page); before, it says what saving will do.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorText, unwrap, type Inbound } from "../../../api/client";
import { qk } from "../../../api/hooks";
import { SwitchRow } from "../../../components/switch";
import { useToast } from "../../../components/toast";
import { t } from "../../../i18n";

export function InboundRUDirect({ inbound, exitNode, exitName }: { inbound?: Inbound | null; exitNode: number; exitName: string }) {
  const saved = !!inbound && inbound.outbound === "node" && inbound.exit_node_id === exitNode;
  const node = inbound?.node_id ?? 0;
  const qc = useQueryClient();
  const toast = useToast();
  const ru = useQuery({
    queryKey: qk.ruDirect(node),
    queryFn: ({ signal }) => unwrap(api.GET("/api/v1/nodes/{id}/cascade/ru-direct", { params: { path: { id: node } }, signal })),
    enabled: saved,
  });
  const set = useMutation({
    mutationFn: (on: boolean) =>
      unwrap(api.PUT("/api/v1/nodes/{id}/cascade/ru-direct", { params: { path: { id: node } }, body: { inbound: inbound!.name, on } })),
    onSuccess: (v) => {
      qc.setQueryData(qk.ruDirect(node), v);
      toast.ok(t("cascade.ruDirect.saved"));
    },
  });

  if (!saved) {
    return <p className="mt-2 text-xs leading-relaxed text-[var(--ink-500)]">{t("inbounds.ruDirect.onSave", { exit: exitName })}</p>;
  }
  const v = ru.data;
  const on = !!v?.inbounds.includes(inbound.name);
  const sub = on
    ? [t("inbounds.ruDirect.on", { exit: exitName }), v?.geodata?.have ? "" : t("inbounds.ruDirect.zonesOnly")].filter(Boolean).join(" ")
    : t("inbounds.ruDirect.off", { exit: exitName });
  return (
    <div className="mt-3">
      {set.error || ru.error ? (
        <div className="banner err mb-2" role="alert">
          {errorText(set.error ?? ru.error)}
        </div>
      ) : null}
      <SwitchRow label={t("cascade.ruDirect.title")} sub={sub} checked={on} disabled={!v || set.isPending} onChange={(x) => set.mutate(x)} />
    </div>
  );
}
