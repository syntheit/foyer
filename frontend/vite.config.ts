import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit(), tailwindcss()],
	server: {
		allowedHosts: ['harbor', 'harbor.tail.matv.io', 'localhost'],
		proxy: {
			'/api': 'http://localhost:8420',
			'/ws': { target: 'ws://localhost:8420', ws: true },
			'/d': 'http://localhost:8420',
			'/p': 'http://localhost:8420'
		}
	}
});
