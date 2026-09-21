import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	// sveltekit() is called bare, no options: passing ANY option here (even
	// unrelated ones) makes Vite ignore svelte.config.js entirely, silently
	// dropping the adapter config below it. Found this the hard way -- the
	// scaffold's default compilerOptions block was enough to trigger it.
	plugins: [sveltekit()],
	server: {
		// The Go backend is a separate process in dev (port 8080); in
		// production one binary serves both, so there's no CORS story at all.
		// Proxying here keeps dev matching that same single-origin shape
		// instead of needing a different one.
		proxy: {
			'/recommendations': { target: 'http://localhost:8080', changeOrigin: true },
			'/in-season': { target: 'http://localhost:8080', changeOrigin: true },
			'/species': { target: 'http://localhost:8080', changeOrigin: true },
			'/hotspots': { target: 'http://localhost:8080', changeOrigin: true }
		}
	}
});
