import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit(),
		{
			name: 'configure-response-headers',
			configureServer: (server) => {
				server.middlewares.use((_req, res, next) => {
					// Required for SharedArrayBuffer (used by SQLite WASM OPFS)
					res.setHeader('Cross-Origin-Opener-Policy', 'same-origin');
					res.setHeader('Cross-Origin-Embedder-Policy', 'require-corp');
					next();
				});
			}
		}
	],
	server: {
		host: '0.0.0.0',
		port: 5173
	},
	optimizeDeps: {
		// sql.js loaded from CDN, fzstd optimized normally
	}
});
