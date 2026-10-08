import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
export default {
	kit: {
		// pages/assets both default to 'build', matching web/build -- already
		// reserved by build/.gitkeep so Go's //go:embed has something to
		// point at even before a real build exists.
		adapter: adapter({ fallback: 'index.html' }),
		// In hash mode SvelteKit adds the hash of each inline script it writes
		// to a <meta> policy in the built pages. Photos come from Wikimedia
		// Commons and iNaturalist, tiles from OpenStreetMap; style attributes
		// need 'unsafe-inline' for styles only.
		csp: {
			mode: 'hash',
			directives: {
				'default-src': ['self'],
				'script-src': ['self'],
				'style-src': ['self', 'unsafe-inline'],
				'img-src': [
					'self',
					'data:',
					'https://*.tile.openstreetmap.org',
					'https://thumb.wikimedia.org',
					'https://upload.wikimedia.org',
					'https://inaturalist-open-data.s3.amazonaws.com'
				],
				'connect-src': ['self'],
				'object-src': ['none'],
				'base-uri': ['self'],
				'form-action': ['self']
			}
		}
	}
};
