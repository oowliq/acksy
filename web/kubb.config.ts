import { adapterOas } from '@kubb/adapter-oas'
import { pluginTs } from '@kubb/plugin-ts'
import { defineConfig } from 'kubb/config'

// API types generated from the server's OpenAPI spec (`pnpm gen:api`).
export default defineConfig({
  input: '../api/openapi.yaml',
  output: { path: './src/gen', clean: true, format: 'biome' },
  // int64 values (event seq) are JSON numbers, not bigint.
  adapter: adapterOas({ integerType: 'number' }),
  plugins: [
    pluginTs({
      output: { path: 'api-types.ts' },
      // String unions, as the API sends plain strings.
      enum: { type: 'literal' },
    }),
  ],
})
