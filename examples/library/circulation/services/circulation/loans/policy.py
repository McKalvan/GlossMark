"""Loan policy. LOAN_PERIOD_DAYS is the value the business sees as lib:LoanPeriod (value-from in circ.gloss.yaml)."""

LOAN_PERIOD_DAYS = 21


# @gloss implements .DefaultLoanPeriod
def due_date(checked_out):
    return checked_out + LOAN_PERIOD_DAYS
