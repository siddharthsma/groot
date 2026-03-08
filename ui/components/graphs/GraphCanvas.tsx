"use client";

import { useMemo } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { layoutGraph } from "@/components/graphs/layout";

const baseNodes: Node[] = [
  {
    id: "event",
    type: "default",
    data: { label: "Inbound Event" },
    position: { x: 0, y: 0 },
    style: { borderRadius: 18, border: "1px solid #cbd5e1", padding: 12 },
  },
  {
    id: "subscription",
    type: "default",
    data: { label: "Subscription Filter" },
    position: { x: 0, y: 0 },
    style: { borderRadius: 18, border: "1px solid #cbd5e1", padding: 12 },
  },
  {
    id: "delivery",
    type: "default",
    data: { label: "Delivery Target" },
    position: { x: 0, y: 0 },
    style: { borderRadius: 18, border: "1px solid #cbd5e1", padding: 12 },
  },
];

const edges: Edge[] = [
  { id: "event-subscription", source: "event", target: "subscription", animated: true },
  { id: "subscription-delivery", source: "subscription", target: "delivery", animated: true },
];

export function GraphCanvas() {
  const nodes = useMemo(() => layoutGraph(baseNodes, edges), []);

  return (
    <div className="h-[360px] overflow-hidden rounded-2xl border border-slate-200 bg-slate-50/80">
      <ReactFlow fitView nodes={nodes} edges={edges}>
        <Background gap={24} color="#cbd5e1" />
        <MiniMap zoomable pannable />
        <Controls />
      </ReactFlow>
    </div>
  );
}
