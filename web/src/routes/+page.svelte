<script lang="ts">
	import { getRecommendations, type Recommendation } from '$lib/api';

	let lat = $state('');
	let lon = $state('');
	let radiusKm = $state('25');
	let date = $state('');

	let recommendations = $state<Recommendation[]>([]);
	let loading = $state(false);
	let error = $state('');

	async function search(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		loading = true;
		try {
			recommendations = await getRecommendations({
				lat: Number(lat),
				lon: Number(lon),
				radiusKm: Number(radiusKm),
				date: date || undefined
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'search failed';
		} finally {
			loading = false;
		}
	}
</script>

<h1>Where to Bird Today</h1>

<form onsubmit={search}>
	<label>
		Latitude
		<input type="number" step="any" bind:value={lat} required />
	</label>
	<label>
		Longitude
		<input type="number" step="any" bind:value={lon} required />
	</label>
	<label>
		Radius (km)
		<input type="number" step="any" bind:value={radiusKm} required />
	</label>
	<label>
		Date
		<input type="date" bind:value={date} />
	</label>
	<button type="submit" disabled={loading}>{loading ? 'Searching…' : 'Search'}</button>
</form>

{#if error}
	<p role="alert">{error}</p>
{/if}

<ul>
	{#each recommendations as hotspot (hotspot.hotspot_id)}
		<li>
			<strong>{hotspot.hotspot_name}</strong> — {hotspot.distance_km.toFixed(1)} km
			<ul>
				{#each hotspot.top_species as species (species.species_id)}
					<li>{species.common_name} ({(species.frequency * 100).toFixed(0)}%)</li>
				{/each}
			</ul>
		</li>
	{/each}
</ul>
