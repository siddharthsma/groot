import { z } from "zod";

export const integrationPlaceholderSchema = z.object({
  name: z.string().min(1),
});
