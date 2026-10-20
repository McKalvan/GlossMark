package lib

// @gloss represents circ:LoanWindow
const val LOAN_WINDOW_DAYS = 21

// @gloss represents circ:RenewalLimit
private const val RENEWALS = 3

object Branch {
    // @gloss represents circ:ShelfCount
    val SHELVES: Int = 12
}

// @gloss implements circ:LoanStore
class LoanStore {
    // @gloss implements circ:MarcFieldMapper
    fun map(field: MarcField) {
    }
}
