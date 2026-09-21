<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import 'leaflet/dist/leaflet.css';
	import type * as Leaflet from 'leaflet';
	import type { Recommendation } from '$lib/api';
	import { checklistCount } from '$lib/format';

	interface Props {
		home: { lat: number; lon: number };
		hotspots: Recommendation[];
		selectedId: number | null;
		onselect: (hotspotId: number) => void;
		onpick: (lat: number, lon: number) => void;
	}

	let { home, hotspots, selectedId, onselect, onpick }: Props = $props();

	const smallestBubble = 40;
	const bubbleRange = 32;
	const phoneQuery = '(max-width: 639px)';
	// The share of a phone's height the bottom sheet covers.
	const sheetShare = 0.45;
	// The space the hotspot card covers: its width on a desktop, the share of the
	// height it may take on a phone.
	const cardWidth = 372;
	const cardShare = 0.6;
	const bubbleClearance = (smallestBubble + bubbleRange) / 2;

	let container: HTMLDivElement;
	let leaflet: typeof Leaflet;
	let map: Leaflet.Map;
	let markersLayer: Leaflet.LayerGroup;
	let homeMarker: Leaflet.Marker | undefined;
	let markers = new Map<number, { marker: Leaflet.Marker; offset: number }>();
	let ready = $state(false);

	// Leaflet touches `window`, so it is loaded here rather than at module
	// scope.
	onMount(() => {
		void (async () => {
			leaflet = await import('leaflet');
			map = leaflet.map(container, { zoomControl: false }).setView([home.lat, home.lon], 9);
			leaflet
				.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
					attribution:
						'&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
				})
				.addTo(map);
			leaflet.control.zoom({ position: 'bottomright' }).addTo(map);
			markersLayer = leaflet.layerGroup().addTo(map);
			map.on('click', (event) => onpick(event.latlng.lat, event.latlng.lng));
			ready = true;
		})();
		return () => map?.remove();
	});

	$effect(() => {
		if (!ready) return;
		const latLng: Leaflet.LatLngTuple = [home.lat, home.lon];
		if (homeMarker) {
			homeMarker.setLatLng(latLng);
			return;
		}
		homeMarker = leaflet
			.marker(latLng, {
				icon: leaflet.divIcon({ className: 'home-icon', iconSize: [40, 40] }),
				interactive: false,
				keyboard: false,
				zIndexOffset: -1000
			})
			.addTo(map);
	});

	$effect(() => {
		if (!ready) return;
		markersLayer.clearLayers();
		markers.clear();
		const largest = Math.max(1, ...hotspots.map(checklistCount));
		for (const hotspot of hotspots) {
			const size = smallestBubble + bubbleRange * Math.sqrt(checklistCount(hotspot) / largest);
			// Smaller bubbles draw above larger ones so none is hidden.
			const offset = Math.round(smallestBubble + bubbleRange - size);
			const marker = leaflet
				.marker([hotspot.latitude, hotspot.longitude], {
					icon: bubbleIcon(hotspot, size),
					title: hotspot.hotspot_name,
					zIndexOffset: offset
				})
				.on('click', () => onselect(hotspot.hotspot_id))
				.addTo(markersLayer);
			markers.set(hotspot.hotspot_id, { marker, offset });
		}
		applySelection(untrack(() => selectedId));
		fitView();
	});

	$effect(() => {
		if (!ready) return;
		applySelection(selectedId);
		const selected = selectedId === null ? undefined : markers.get(selectedId);
		if (!selected) return;
		// The hotspot card covers part of the map, so keep the whole bubble clear
		// of it, not only its centre.
		const phone = window.matchMedia(phoneQuery).matches;
		map.panInside(selected.marker.getLatLng(), {
			paddingTopLeft: [bubbleClearance, (phone ? 80 : 20) + bubbleClearance],
			paddingBottomRight: [
				(phone ? 20 : cardWidth) + bubbleClearance,
				(phone ? window.innerHeight * cardShare : 20) + bubbleClearance
			]
		});
	});

	function applySelection(id: number | null) {
		for (const [hotspotId, { marker, offset }] of markers) {
			const selected = hotspotId === id;
			marker.getElement()?.classList.toggle('selected', selected);
			marker.setZIndexOffset(selected ? offset + 1000 : offset);
		}
	}

	// The names come from outside, so the marker is built from elements and
	// text nodes, never from an HTML string.
	function bubbleIcon(hotspot: Recommendation, size: number): Leaflet.DivIcon {
		const bubble = document.createElement('div');
		bubble.className = 'bubble';
		bubble.textContent = String(checklistCount(hotspot));
		const label = document.createElement('span');
		label.className = 'bubble-label';
		label.textContent = hotspot.hotspot_name;
		const wrapper = document.createElement('div');
		wrapper.style.width = wrapper.style.height = '100%';
		wrapper.append(bubble, label);
		return leaflet.divIcon({
			className: 'bubble-icon',
			html: wrapper,
			iconSize: [size, size],
			iconAnchor: [size / 2, size / 2]
		});
	}

	function fitView() {
		const start = untrack(() => ({ lat: home.lat, lon: home.lon }));
		const points: Leaflet.LatLngTuple[] = hotspots.map((hotspot) => [hotspot.latitude, hotspot.longitude]);
		if (points.length === 0) {
			map.setView([start.lat, start.lon], map.getZoom());
			return;
		}
		const phone = window.matchMedia(phoneQuery).matches;
		map.fitBounds(leaflet.latLngBounds([...points, [start.lat, start.lon]]), {
			paddingTopLeft: [bubbleClearance, (phone ? 80 : 20) + bubbleClearance],
			paddingBottomRight: [bubbleClearance, (phone ? window.innerHeight * sheetShare : 20) + bubbleClearance],
			maxZoom: 13
		});
	}
</script>

<div bind:this={container} class="map"></div>

<style>
	.map {
		width: 100%;
		height: 100%;
		isolation: isolate;
	}

	/* Fixed, and below the bottom sheet and the hotspot card, which are fixed too;
	   all of them then stack against each other in one context. */
	@media (max-width: 639px) {
		.map {
			position: fixed;
			inset: 0;
			z-index: 0;
		}
	}
</style>
