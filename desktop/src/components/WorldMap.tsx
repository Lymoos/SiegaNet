import { useEffect, useRef, useState } from "react";
import {
  ComposableMap,
  Geographies,
  Geography,
  Marker,
  ZoomableGroup,
} from "react-simple-maps";
import worldTopo from "world-atlas/countries-110m.json";
import type { Server, Status } from "../api/types";
import { coordsFor } from "../geo/serverGeo";
import { loadColor } from "../lib/flags";
import { flagUrlFor } from "../lib/flagAssets";
import { ServerCard } from "./ServerCard";

interface Props {
  servers: Server[];
  status: Status;
  selectedId: string | null;
  onSelect: (serverId: string | null) => void;
  onConnect: (serverId: string) => void;
  onDisconnect: () => void;
}

/**
 * Real world map (Natural Earth 110m via world-atlas topojson, rendered by
 * react-simple-maps) styled to the SiegaNet dark theme.
 *
 * Server pins: dark disc with the ISO country code inside; the ring colour
 * encodes load_pct (green = free → orange → red = loaded), so relative
 * server pressure reads at a glance. Click opens the server card anchored
 * near the pin; the card dismisses on outside click and Escape.
 */
export function WorldMap({
  servers,
  status,
  selectedId,
  onSelect,
  onConnect,
  onDisconnect,
}: Props) {
  const wrapRef = useRef<HTMLDivElement>(null);
  // card anchor in container px; null => default corner position
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);

  const selected = servers.find((s) => s.id === selectedId) ?? null;

  // selection can arrive from the sidebar (no click point) — drop stale anchor
  useEffect(() => {
    if (!selectedId) setAnchor(null);
  }, [selectedId]);

  // Escape closes the card
  useEffect(() => {
    if (!selectedId) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onSelect(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selectedId, onSelect]);

  const markerClick = (id: string, e: React.MouseEvent) => {
    e.stopPropagation(); // don't let the outside-click handler swallow it
    const rect = wrapRef.current?.getBoundingClientRect();
    if (rect) setAnchor({ x: e.clientX - rect.left, y: e.clientY - rect.top });
    onSelect(id);
  };

  return (
    /* any click that reaches the container is "outside": map background,
       countries, ocean — the card and the pins stop propagation */
    <div className="map-wrap" ref={wrapRef} onClick={() => onSelect(null)}>
      <ComposableMap
        projection="geoNaturalEarth1"
        projectionConfig={{ scale: 172, center: [12, 8] }}
        width={980}
        height={520}
        style={{ width: "100%", height: "100%" }}
      >
        <ZoomableGroup minZoom={1} maxZoom={5}>
          <Geographies geography={worldTopo}>
            {({ geographies }) =>
              geographies
                .filter((geo) => geo.properties.name !== "Antarctica")
                .map((geo) => (
                  <Geography key={geo.rsmKey} geography={geo} className="geo" />
                ))
            }
          </Geographies>

          {servers.map((s) => {
            const coords = coordsFor(s.city);
            if (!coords) return null;
            const isActive = status.server_id === s.id;
            const markerState =
              isActive && status.state === "connected"
                ? "connected"
                : isActive && status.state === "connecting"
                  ? "connecting"
                  : "";
            const isSelected = selectedId === s.id;
            const r = isSelected ? 8.5 : 7;
            const url = flagUrlFor(s.country);
            const clipId = `pin-clip-${s.id}`;
            return (
              <Marker
                key={s.id}
                coordinates={coords}
                className={`marker ${markerState} ${isSelected ? "is-selected" : ""}`}
                onClick={(e: React.MouseEvent) => markerClick(s.id, e)}
              >
                {/* pulse ring for connected/connecting */}
                <circle className="marker-halo" r={8} />
                {/* selection glow behind the pin */}
                {isSelected && <circle className="pin-glow" r={11.5} />}
                {/* pin: real flag cropped to a centred circle + load ring */}
                {url ? (
                  <>
                    <defs>
                      <clipPath id={clipId}>
                        <circle r={r} />
                      </clipPath>
                    </defs>
                    <image
                      href={url}
                      x={-r}
                      y={-r}
                      width={2 * r}
                      height={2 * r}
                      preserveAspectRatio="xMidYMid slice"
                      clipPath={`url(#${clipId})`}
                    />
                  </>
                ) : (
                  <circle className="pin-body" r={r} />
                )}
                <circle
                  className="pin-ring"
                  r={r}
                  style={{ stroke: loadColor(s.load_pct) }}
                />
                <title>{`${s.country}, ${s.city} — ${s.ping_ms} ms · нагрузка ${s.load_pct}%`}</title>
              </Marker>
            );
          })}
        </ZoomableGroup>
      </ComposableMap>

      <div className="map-legend">
        <span className="legend-title">нагрузка</span>
        <span className="legend-swatch" style={{ background: loadColor(15) }} />
        <span>низкая</span>
        <span className="legend-swatch" style={{ background: loadColor(55) }} />
        <span>средняя</span>
        <span className="legend-swatch" style={{ background: loadColor(90) }} />
        <span>высокая</span>
      </div>
      <div className="map-hint">колесо — масштаб · перетаскивание — сдвиг · клик по пину — сервер</div>

      {selected && (
        <ServerCard
          server={selected}
          status={status}
          anchor={anchor}
          containerRef={wrapRef}
          onConnect={onConnect}
          onDisconnect={onDisconnect}
          onClose={() => onSelect(null)}
        />
      )}
    </div>
  );
}
