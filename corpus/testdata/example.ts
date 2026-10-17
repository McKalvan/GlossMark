// @gloss? represents zoo:Mare conf=0.72
export interface Mare { id: string; foalingDate?: Date }

/* @gloss implements impl:FoalRegistry */
export class FoalRegistry {
  register(foal: Foal) {}
}

const msg = "// @gloss implements impl:Nothing"; // inside a string, not a marker

/**
 * Multi-line doc comment.
 * @gloss documents zoo:Foal
 */
export type Foal = { id: string };
