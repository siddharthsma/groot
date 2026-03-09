export type GraphNodeData = {
  label: string;
  kind: "event" | "subscription" | "connection";
};

export type GraphLayoutNode = {
  id: string;
  data: GraphNodeData;
  position: { x: number; y: number };
};
