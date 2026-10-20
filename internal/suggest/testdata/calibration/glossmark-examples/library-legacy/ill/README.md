<!-- @gloss documents lib:InterlibraryLoan scope=file conf=0.85 -->
# Interlibrary loan (being retired)

CIRC handled interlibrary loan requests from 2013 to 2025: patrons asked for a title Riverbend does not own, staff borrowed it from another library, and CIRC lent it like a local item.

In 2025 the city joined the Prairie Libraries Consortium. Request intake moved to the consortium's portal and the `IllRequestServlet` and `ill_requests` tables were removed from CIRC (CIRC-2210). The city's service catalogue still lists interlibrary loan as a CIRC service until the contract hand-over on 30 June 2027; until then this directory keeps the consortium endpoint that desk staff use to look up a request.

What is left here:

- `config/ill.yaml`: the consortium endpoint and the branch delivery codes.
