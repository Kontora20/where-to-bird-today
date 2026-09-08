import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
export default {
	kit: {
		// pages/assets both default to 'build', matching web/build -- already
		// reserved by build/.gitkeep so Go's //go:embed has something to
		// point at even before a real build exists.
		adapter: adapter({ fallback: 'index.html' })
	}
};
