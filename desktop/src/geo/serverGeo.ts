/**
 * City -> [lon, lat] lookup for placing server dots on the real world map.
 *
 * The control contract (GET /servers) does not carry coordinates, so the UI
 * resolves them from the city name. If a city is unknown the server is still
 * fully usable from the sidebar list — it just has no dot on the map.
 *
 * «Нужен endpoint в ядре» (nice-to-have): add lat/lon (and an ISO country
 * code) to GET /servers, then this table becomes a fallback only.
 */

const CITY_COORDS: Record<string, [number, number]> = {
  "Амстердам": [4.9041, 52.3676],
  "Франкфурт": [8.6821, 50.1109],
  "Хельсинки": [24.9384, 60.1699],
  "Стокгольм": [18.0686, 59.3293],
  "Лондон": [-0.1276, 51.5072],
  "Париж": [2.3522, 48.8566],
  "Варшава": [21.0122, 52.2297],
  "Стамбул": [28.9784, 41.0082],
  "Алматы": [76.8897, 43.2389],
  "Дубай": [55.2708, 25.2048],
  "Сингапур": [103.8198, 1.3521],
  "Токио": [139.6917, 35.6895],
  "Нью-Йорк": [-74.006, 40.7128],
  "Лос-Анджелес": [-118.2437, 34.0522],
  "Сан-Паулу": [-46.6333, -23.5505],
};

export function coordsFor(city: string): [number, number] | null {
  return CITY_COORDS[city] ?? null;
}
