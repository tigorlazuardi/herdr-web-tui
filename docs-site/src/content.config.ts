import { defineCollection, z } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';

export const collections = {
  docs: defineCollection({
    loader: docsLoader(),
    schema: docsSchema({
      extend: z.object({
        status: z.enum(['draft', 'accepted', 'superseded']).optional(),
        date: z.coerce.date().optional(),
        severity: z.enum(['info', 'low', 'medium', 'high', 'critical']).optional(),
        tags: z.array(z.string()).optional(),
      }),
    }),
  }),
};
