export function useApiBaseUrl() {
  return (
    process.env.NEXT_PUBLIC_GROOT_API_BASE_URL?.replace(/\/$/, "") ??
    "http://localhost:8081"
  );
}
