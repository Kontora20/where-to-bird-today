export interface Place {
	label: string;
	lat: number;
	lon: number;
}

// The coordinates are those of warmupOrigins in cmd/server/main.go, so a search
// from one of these is answered from the server's drive-time cache.
export const cities: Place[] = [
	{ label: 'Vilnius', lat: 54.6872, lon: 25.2797 },
	{ label: 'Kaunas', lat: 54.8985, lon: 23.9036 },
	{ label: 'Klaipėda', lat: 55.7033, lon: 21.1443 },
	{ label: 'Šiauliai', lat: 55.9349, lon: 23.314 },
	{ label: 'Panevėžys', lat: 55.7333, lon: 24.3667 }
];
