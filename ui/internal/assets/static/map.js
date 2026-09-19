// Aircraft map for the aircraft display, rendered with the pinned Leaflet
// 1.9.4 ES module from the asset base.
//
// One map is created per mounted display from the configured center and
// zoom, and it is never re-centered by data: polling, selection changes,
// station edits and outage recovery keep the user's viewport. Only the
// explicit "Fit received aircraft" action changes it.
//
// Aircraft markers are keyed by run ID and ICAO and exist only for a valid
// received position on a track that is not lost; they are moved in place.
// A direction arrow is drawn only when a received ground track exists.
// Stations come from display discovery; each selected enabled station gets
// a circle of its published effective radius (nautical miles times 1852
// metres). Circles are synthetic reception-model visualizations, not
// navigation or RF guarantees; the legend says so and names the reference
// pressure altitude.
//
// Tiles are added only when a complete tiles object is configured; with
// tiles null the map requests no tile at all. Tile failures are reported
// separately and never affect received data. Leaflet's attribution control
// is disabled because it renders HTML; the configured attribution is shown
// as text with a validated link instead. Nothing here uses innerHTML.

import {
  circle, circleMarker, divIcon, latLngBounds, layerGroup, map as createMap, marker, tileLayer,
} from "./leaflet/leaflet-src.esm.js";
import { h, replaceChildren, setText } from "./dom.js";
import { clippedByProjection, coverageMetres, normalizeLongitude } from "./geo.js";

function aircraftIcon(row, stale) {
  const hasTrack = row.velocity !== null && row.velocity.trackDegrees !== null;
  const glyph = h("span", { class: `tb-aircraft-icon${stale ? " tb-stale-marker" : ""}`, "aria-hidden": "true" },
    hasTrack ? "\u2191" : "\u25cf");
  if (hasTrack) {
    glyph.style.transform = `rotate(${row.velocity.trackDegrees}deg)`;
  }
  return divIcon({ html: glyph, className: "tb-aircraft-marker", iconSize: [20, 20], iconAnchor: [10, 10] });
}

function markerTitle(row) {
  const callsign = row.identity ? ` ${row.identity.callsign}` : "";
  const state = row.status === "stale" ? " (stale)" : "";
  return `Aircraft ${row.icao}${callsign}${state}`;
}

/**
 * Creates the map inside panel. options: { lifecycle, config, onSelect }.
 * Returns { update(model), destroy() } where model is
 * { view, catalog, selection, selectedIcao, stale }.
 */
export function createAircraftMap(panel, { lifecycle, config, onSelect }) {
  const container = h("div", { class: "tb-map", role: "region", "aria-label": "Aircraft map" });
  const fitButton = h("button", { type: "button", disabled: true }, "Fit received aircraft");
  const tileStatus = h("p", { class: "tb-muted", role: "status", "aria-live": "polite" });
  const attribution = h("p", { class: "tb-attribution" });
  const legend = h("ul", { class: "tb-legend" });
  const clipping = h("p", { class: "tb-muted" });
  panel.append(h("div", { class: "tb-actions" }, fitButton), container, attribution, tileStatus, clipping,
    h("h3", {}, "Coverage"), legend);

  const tiles = config.tiles;
  const leaflet = createMap(container, {
    center: [config.initialLatitudeDegrees, config.initialLongitudeDegrees],
    zoom: config.initialZoom,
    minZoom: tiles ? tiles.minZoom : 0,
    maxZoom: tiles ? tiles.maxZoom : 24,
    attributionControl: false,
    worldCopyJump: false,
  });
  const publishViewport = () => {
    const center = leaflet.getCenter();
    container.dataset.tbCenter = `${center.lat.toFixed(6)},${center.lng.toFixed(6)}`;
    container.dataset.tbZoom = String(leaflet.getZoom());
  };
  leaflet.on("moveend zoomend", publishViewport);
  publishViewport();

  if (tiles) {
    const layer = tileLayer(tiles.urlTemplate, { minZoom: tiles.minZoom, maxZoom: tiles.maxZoom });
    let failed = 0;
    layer.on("tileerror", () => {
      failed += 1;
      setText(tileStatus, `Map tiles are unavailable (${failed} failed). Received data, selection and inspection are unaffected.`);
    });
    layer.on("tileload", () => {
      if (failed === 0) {
        setText(tileStatus, "");
      }
    });
    layer.addTo(leaflet);
    const link = h("a", { href: tiles.attributionUrl, rel: "noopener noreferrer", target: "_blank" }, tiles.attributionText);
    replaceChildren(attribution, "Map tiles: ", link);
  } else {
    setText(attribution, "Map tiles are disabled; positions are drawn without a base map.");
  }

  const stationLayer = layerGroup().addTo(leaflet);
  const coverageLayer = layerGroup().addTo(leaflet);
  const aircraftLayer = layerGroup().addTo(leaflet);
  const markers = new Map();

  lifecycle.listen(fitButton, "click", () => {
    const points = [...markers.values()].map((entry) => entry.marker.getLatLng());
    if (points.length > 0) {
      leaflet.fitBounds(latLngBounds(points), { padding: [24, 24], maxZoom: tiles ? tiles.maxZoom : 12 });
    }
  });

  function updateAircraft(view, selectedIcao, stale) {
    const wanted = new Map();
    for (const row of view?.rows ?? []) {
      if (row.tombstone || row.position === null || row.status === "lost") {
        continue;
      }
      wanted.set(`${view.runId}:${row.icao}`, row);
    }
    for (const [key, entry] of markers) {
      if (!wanted.has(key)) {
        aircraftLayer.removeLayer(entry.marker);
        markers.delete(key);
      }
    }
    const clipped = [];
    for (const [key, row] of wanted) {
      const position = [row.position.latitudeDegrees, normalizeLongitude(row.position.longitudeDegrees)];
      const markerStale = stale || row.status === "stale";
      const signature = `${row.velocity?.trackDegrees ?? "none"}:${markerStale}:${row.icao === selectedIcao}`;
      let entry = markers.get(key);
      if (!entry) {
        const created = marker(position, { icon: aircraftIcon(row, markerStale), title: markerTitle(row), keyboard: true, alt: markerTitle(row) });
        created.on("click", () => onSelect(row.icao));
        created.addTo(aircraftLayer);
        entry = { marker: created, signature };
        markers.set(key, entry);
      } else {
        entry.marker.setLatLng(position);
        if (entry.signature !== signature) {
          entry.marker.setIcon(aircraftIcon(row, markerStale));
          entry.signature = signature;
        }
      }
      const element = entry.marker.getElement();
      if (element) {
        element.classList.toggle("tb-selected-marker", row.icao === selectedIcao);
        element.dataset.icao = row.icao;
        element.setAttribute("title", markerTitle(row));
        element.setAttribute("aria-label", markerTitle(row));
      }
      if (clippedByProjection(row.position.latitudeDegrees)) {
        clipped.push(row.icao);
      }
    }
    fitButton.disabled = markers.size === 0;
    setText(clipping, clipped.length > 0
      ? `Drawn clipped at the map projection limit (exact coordinates in the table): ${clipped.join(", ")}.`
      : "");
  }

  function updateStations(catalog, selection) {
    stationLayer.clearLayers();
    coverageLayer.clearLayers();
    const items = [];
    for (const { station, coverage } of catalog?.stations ?? []) {
      const position = [station.latitudeDegrees, normalizeLongitude(station.longitudeDegrees)];
      const selected = selection.includes(station.id);
      const dot = circleMarker(position, {
        radius: 6, weight: 2, color: station.enabled ? "#0969da" : "#636c76",
        dashArray: station.enabled ? null : "3 3", fillOpacity: station.enabled ? 0.8 : 0.2,
      });
      dot.bindTooltip(h("span", {}, `${station.id} (${station.enabled ? "enabled" : "disabled"})`));
      dot.addTo(stationLayer);
      const radii = `effective radius ${coverage.effectiveRadiusNauticalMiles.toFixed(1)} NM ` +
        `(horizon ${coverage.horizonRadiusNauticalMiles.toFixed(1)} NM, link budget ${coverage.linkBudgetRadiusNauticalMiles.toFixed(1)} NM)`;
      if (selected && station.enabled) {
        circle(position, {
          radius: coverageMetres(coverage.effectiveRadiusNauticalMiles), weight: 1, color: "#0969da", fillOpacity: 0.05,
          interactive: false, className: "tb-coverage",
        }).addTo(coverageLayer);
        items.push(h("li", { "data-station": station.id },
          `${station.id}: ${radii} at reference pressure altitude ${coverage.referenceAltitudeFeet} ft. Synthetic reception-model estimate, not measured coverage.`));
      } else {
        items.push(h("li", { "data-station": station.id, class: "tb-muted" },
          `${station.id}: ${station.enabled ? "not selected" : "disabled"}; no coverage drawn.`));
      }
    }
    replaceChildren(legend, ...items);
  }

  let destroyed = false;
  return {
    update({ view, catalog, selection, selectedIcao, stale }) {
      if (destroyed) {
        return;
      }
      updateAircraft(view, selectedIcao, stale);
      updateStations(catalog, selection);
    },
    destroy() {
      if (destroyed) {
        return;
      }
      destroyed = true;
      leaflet.off();
      leaflet.remove();
      markers.clear();
    },
  };
}
