// @gloss? represents lib:Book conf=0.72
export interface Book { id: string; dueDate?: Date }

/* @gloss implements circ:LoanStore */
export class LoanStore {
  register(loan: Loan) {}
}

const msg = "// @gloss implements circ:Nothing"; // inside a string, not a marker

/**
 * Multi-line doc comment.
 * @gloss documents lib:Loan
 */
export type Loan = { id: string };
