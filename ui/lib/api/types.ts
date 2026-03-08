export type GrootHealthResponse = {
  status: string;
};

export type RequestOptions = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  headers?: HeadersInit;
  body?: BodyInit | null;
  signal?: AbortSignal;
};
