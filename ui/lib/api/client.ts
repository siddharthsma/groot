import type { RequestOptions } from "@/lib/api/types";

function getBaseURL() {
  return (
    process.env.NEXT_PUBLIC_GROOT_API_BASE_URL?.replace(/\/$/, "") ??
    "http://localhost:8081"
  );
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}) {
  const response = await fetch(`${getBaseURL()}${path}`, {
    ...options,
    headers: {
      Accept: "application/json",
      ...options.headers,
    },
  });

  if (!response.ok) {
    throw new Error(`API request failed with status ${response.status}`);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}
