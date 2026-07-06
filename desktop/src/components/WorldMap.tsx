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
 * react-simple-maps) styled to the SiegaNet dark theme. Server dots are
 * clickable; a click opens the server card anchored near the dot.
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

  const markerClick = (id: string, e: React.MouseEvent) => {
    const rect = wrapRef.current?.getBoundingClientRect();
    if (rect) setAnchor({ x: e.clientX - rect.left, y: e.clientY - rect.top });
    onSelect(id);
  };

  return (
    <div className="map-wrap" ref={wrapRef}>
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
            return (
              <Marker
                key={s.id}
                coordinates={coords}
                className={`marker ${markerState}`}
                onClick={(e: React.MouseEvent) => markerClick(s.id, e)}
              >
                <circle className="marker-halo" r={5} />
                <circle
                  className="marker-dot"
                  r={selectedId === s.id ? 5 : 3.6}
                >
                  <title>{`${s.country}, ${s.city} — ${s.ping_ms} ms`}</title>
                </circle>
              </Marker>
            );
          })}
        </ZoomableGroup>
      </ComposableMap>

      <div className="map-hint">колесо — масштаб · перетаскивание — сдвиг · клик по точке — сервер</div>

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
