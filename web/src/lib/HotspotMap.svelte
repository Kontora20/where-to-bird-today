<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import 'leaflet/dist/leaflet.css';
	// Only the base stylesheet: MarkerCluster.Default.css styles the plugin's
	// own built-in coloured-circle icon, which iconCreateFunction below
	// replaces entirely, so importing it would just be unused CSS.
	import 'leaflet.markercluster/dist/MarkerCluster.css';
	import type * as Leaflet from 'leaflet';
	import type { Recommendation, SpeciesMatch } from '$lib/api';
	import { birdBadge, birdCount, checklistCount, type MapLayer } from '$lib/format';

	interface Props {
		home: { lat: number; lon: number };
		hotspots: Recommendation[];
		selectedId: number | null;
		birds: SpeciesMatch[];
		layer: MapLayer;
		onselect: (hotspotId: number) => void;
		onpick: (lat: number, lon: number) => void;
	}

	let { home, hotspots, selectedId, birds, layer, onselect, onpick }: Props = $props();

	const today = new Date();

	const smallestBubble = 19;
	const bubbleRange = 13;
	const phoneQuery = '(max-width: 639px)';
	// The share of a phone's height the bottom sheet covers.
	const sheetShare = 0.45;
	const bubbleClearance = (smallestBubble + bubbleRange) / 2;

	// The pixel radius markers are grouped within, at any zoom: smaller than
	// Leaflet.markercluster's own default (80) so a cluster only forms once
	// bubbles would otherwise overlap, not whenever they're merely close.
	const maxClusterRadius = 35;

	// Bubbles are small while the whole area is in view and reach full size only
	// when zoomed right in, so the map stays clear. Zoom levels from the first
	// number are drawn at the matching share of full size.
	const bubbleScales: [zoom: number, scale: number][] = [[14, 1], [13, 0.85], [12, 0.65]];
	const farBubbleScale = 0.45;
	// Below this the number would be unreadable, so it is left out.
	const smallestReadableScale = 0.75;

	function bubbleScaleAt(zoom: number): number {
		return bubbleScales.find(([fromZoom]) => zoom >= fromZoom)?.[1] ?? farBubbleScale;
	}

	function applyBubbleScale() {
		const scale = bubbleScaleAt(map.getZoom());
		container.style.setProperty('--bubble-scale', String(scale));
		container.style.setProperty('--bubble-number', scale >= smallestReadableScale ? 'var(--color-ground)' : 'transparent');
	}

	let container: HTMLDivElement;
	let leaflet: typeof Leaflet;
	let map: Leaflet.Map;
	let markersLayer: Leaflet.MarkerClusterGroup;
	let homeMarker: Leaflet.Marker | undefined;
	let markers = new Map<number, { marker: Leaflet.Marker; offset: number }>();
	let resizeObserver: ResizeObserver | undefined;
	let ready = $state(false);

	// Leaflet touches `window`, so it is loaded here rather than at module
	// scope.
	onMount(() => {
		void (async () => {
			leaflet = await import('leaflet');
			// leaflet.markercluster predates bundlers: it never imports Leaflet
			// itself, it just extends a global `L` it assumes a <script> tag
			// already created. This is that global, standing in for the tag.
			(window as unknown as { L: typeof Leaflet }).L = leaflet;
			await import('leaflet.markercluster');
			map = leaflet.map(container, { zoomControl: false }).setView([home.lat, home.lon], 9);
			leaflet
				.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
					attribution:
						'&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
				})
				.addTo(map);
			leaflet.control.zoom({ position: 'bottomright' }).addTo(map);
			leaflet.control.scale({ position: 'bottomleft', imperial: false }).addTo(map);
			markersLayer = leaflet
				.markerClusterGroup({
					maxClusterRadius: (zoom) => Math.max(14, Math.round(maxClusterRadius * bubbleScaleAt(zoom))),
					showCoverageOnHover: false,
					iconCreateFunction: (cluster) => clusterIcon(cluster.getAllChildMarkers())
				})
				.addTo(map);
			map.on('zoomend', applyBubbleScale);
			applyBubbleScale();
			map.on('click', (event) => {
				// The map repeats across the date line; the API takes -180 to 180.
				const point = event.latlng.wrap();
				onpick(point.lat, point.lng);
			});
			// The frame, the caption and the year strip settle the map's size after
			// Leaflet has measured it, and a window resize changes it later; the
			// map keeps its centre when it re-measures.
			resizeObserver = new ResizeObserver(() => map.invalidateSize());
			resizeObserver.observe(container);
			ready = true;
		})();
		return () => {
			resizeObserver?.disconnect();
			map?.remove();
		};
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
				icon: leaflet.divIcon({ className: 'home-icon', iconSize: [34, 34] }),
				interactive: false,
				keyboard: false,
				zIndexOffset: 1000
			})
			.addTo(map);
	});

	$effect(() => {
		if (!ready) return;
		markersLayer.clearLayers();
		markers.clear();
		const sizeOf = layer === 'yourBirds' ? birdCount : checklistCount;
		const largest = Math.max(1, ...hotspots.map(sizeOf));
		for (const [rank, hotspot] of hotspots.entries()) {
			// A hotspot with nothing to show in this layer -- no checklists this
			// week, or none of your birds -- draws no bubble; it still shows up
			// in the list, but a map full of same-size "0" bubbles is noise, not
			// information.
			if (sizeOf(hotspot) === 0) continue;
			const size = smallestBubble + bubbleRange * Math.sqrt(sizeOf(hotspot) / largest);
			// Smaller bubbles draw above larger ones so none is hidden.
			const offset = Math.round(smallestBubble + bubbleRange - size);
			const marker = leaflet
				.marker([hotspot.latitude, hotspot.longitude], {
					icon: bubbleIcon(hotspot, size, rank + 1),
					title: hotspot.hotspot_name,
					zIndexOffset: offset,
					...{ rank: rank + 1, recent: isRecent(hotspot) }
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
		const id = selectedId;
		applySelection(id);
		const selected = id === null ? undefined : markers.get(id);
		if (!selected) return;
		// A selected bubble may be sitting inside a collapsed cluster; this
		// zooms in only as far as needed to split it back out, a no-op if it is
		// already showing on its own. applySelection runs again once it does,
		// since the marker had no DOM element to style before this.
		markersLayer.zoomToShowLayer(selected.marker, () => {
			applySelection(id);
			// On a phone the bottom sheet covers the lower part of the map, so keep
			// the whole bubble clear of it, not only its centre.
			const phone = window.matchMedia(phoneQuery).matches;
			map.panInside(selected.marker.getLatLng(), {
				paddingTopLeft: [bubbleClearance, (phone ? 80 : 20) + bubbleClearance],
				paddingBottomRight: [
					bubbleClearance,
					(phone ? window.innerHeight * sheetShare : 20) + bubbleClearance
				]
			});
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
	// text nodes, never from an HTML string. The number is the hotspot's place
	// in the list.
	function bubbleIcon(hotspot: Recommendation, size: number, number: number): Leaflet.DivIcon {
		const bubble = document.createElement('div');
		bubble.className = 'bubble';
		bubble.textContent = String(number);
		const label = document.createElement('span');
		label.className = 'bubble-label';
		label.textContent = hotspot.hotspot_name;
		const wrapper = document.createElement('div');
		wrapper.style.width = wrapper.style.height = '100%';
		wrapper.append(bubble, label);
		return leaflet.divIcon({
			className: isRecent(hotspot) ? 'bubble-icon recent' : 'bubble-icon',
			html: wrapper,
			iconSize: [size, size],
			iconAnchor: [size / 2, size / 2]
		});
	}

	function isRecent(hotspot: Recommendation): boolean {
		return birdBadge(hotspot, birds, today)?.recent ?? false;
	}

	// A group of hotspots shows the number of the best-ranked one inside it, so
	// every number on the map means a place in the list. The outer ring marks it
	// as a group; zooming in splits it into the numbered hotspots.
	function clusterIcon(children: Leaflet.Marker[]): Leaflet.DivIcon {
		const members = children.map((child) => child.options as unknown as { rank: number; recent: boolean });
		const size = Math.min(smallestBubble + bubbleRange, smallestBubble + 6 * Math.sqrt(members.length));
		const cluster = document.createElement('div');
		cluster.className = 'cluster';
		cluster.textContent = String(Math.min(...members.map((member) => member.rank)));
		return leaflet.divIcon({
			className: members.some((member) => member.recent) ? 'cluster-icon recent' : 'cluster-icon',
			html: cluster,
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
	/* Leaflet sets position: relative itself only when the container has none,
	   so a map first drawn in the phone's fixed layout would be static after
	   a resize to the desktop layout. */
	.map {
		position: relative;
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
