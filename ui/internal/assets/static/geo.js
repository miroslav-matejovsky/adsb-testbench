// DOM-free map geometry helpers. They never change source coordinates; they
// only decide how a coordinate is drawn.

/** Metres per nautical mile, exact by definition. */
export const METRES_PER_NAUTICAL_MILE = 1852;

/** Latitude limit of the Web Mercator projection used by the map. */
export const MERCATOR_LATITUDE_LIMIT = 85.0511287798066;

/** Normalizes a longitude into [-180, 180) without changing its meaning. */
export function normalizeLongitude(longitude) {
  const wrapped = ((((longitude + 180) % 360) + 360) % 360) - 180;
  return Object.is(wrapped, -0) ? 0 : wrapped;
}

/** Reports whether a latitude is drawn clipped by the projection. */
export function clippedByProjection(latitude) {
  return Math.abs(latitude) > MERCATOR_LATITUDE_LIMIT;
}

/** Converts a coverage radius in nautical miles to metres. */
export function coverageMetres(nauticalMiles) {
  return nauticalMiles * METRES_PER_NAUTICAL_MILE;
}
