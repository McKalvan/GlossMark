<!-- @gloss documents lib:FineAssessment, lib:GracePeriod, lib:FineCap, lib:PatronBlock scope=file conf=0.85 -->
# Fines policy (Circulation policy 4.2)

Approved by the Library Board, March 2018. CIRC implements it in `FineCalculator.java` and `sp_assess_fines`; the numbers are in `config/circulation.yaml`.

- No fine accrues during a **grace period** of three days after the due date.
- After that, the daily rate is $0.25, or $1.00 for DVDs and $0.10 for magazines.
- Juvenile cards pay no overdue fines. Seniors pay half.
- A fine for one item is **capped** at the item's replacement cost or $20.00, whichever is lower.
- A patron who owes $10.00 or more is **blocked** from borrowing until they pay below that.
- An item 60 days overdue is declared **lost** and billed at replacement cost.
