import { z } from "zod";

export const providerPlaceholderSchema = z.object({
  name: z.string().min(1),
});
