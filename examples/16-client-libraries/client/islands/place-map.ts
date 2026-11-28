// The place-map island: a Leaflet map inside a lit element. Leaflet's JS is
// loaded with import() on first mount, so it is a chunk of its own that
// only pages rendering the island fetch. Its CSS joins the island's
// stylesheet, which the runtime links when the island mounts.

import "leaflet/dist/leaflet.css";
import { html } from "lit";
import type { Map as LeafletMap } from "leaflet";
import { PlaceMapBase } from "templatic/islands/place-map.base";

customElements.define(
	"place-map",
	class extends PlaceMapBase {
		map: LeafletMap | undefined;

		protected render(): unknown {
			return html`<div class="map" role="img" aria-label="Map of the offices"></div>`;
		}

		protected async firstUpdated(): Promise<void> {
			// leaflet is a UMD module: its API is the default export.
			const { default: L } = await import("leaflet");
			if (!this.isConnected || !this.props) return;
			// No tile layer: the markers on a blank map need no tile server.
			const map = (this.map = L.map(this.querySelector<HTMLElement>(".map")!));
			const points = this.props.places.map((p) => {
				L.circleMarker([p.lat, p.lng], { radius: 8 }).bindTooltip(p.name, { permanent: true }).addTo(map);
				return L.latLng(p.lat, p.lng);
			});
			map.fitBounds(L.latLngBounds(points), { padding: [40, 40] });
		}

		disconnectedCallback(): void {
			super.disconnectedCallback();
			this.map?.remove();
			this.map = undefined;
		}
	},
);
