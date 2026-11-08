import { Entity } from '@backstage/catalog-model';

/** The URL of the entity's boundary document (`<base>/<slug>.json`). */
export const EXPORT_URL_ANNOTATION = 'glossmark.dev/export-url';

/**
 * The entity's boundary, as its CURIE (`claimb:assessment`) or slug
 * (`claimb-assessment`); the document is `glossmark.baseUrl/<slug>.json`.
 */
export const BOUNDARY_ANNOTATION = 'glossmark.dev/boundary';

export function exportUrlOf(entity: Entity): string | undefined {
  return entity.metadata.annotations?.[EXPORT_URL_ANNOTATION];
}

export function boundaryOf(entity: Entity): string | undefined {
  return entity.metadata.annotations?.[BOUNDARY_ANNOTATION];
}

/** True when the entity carries either annotation: the tab's `if=`. */
export function isGlossMarkAvailable(entity: Entity): boolean {
  return Boolean(exportUrlOf(entity) || boundaryOf(entity));
}
