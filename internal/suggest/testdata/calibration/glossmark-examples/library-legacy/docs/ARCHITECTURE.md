# CIRC architecture

CIRC is Riverbend Public Library's circulation system. It was written in 2010 and 2011 by the city's IT department to replace a vendor system, and has been extended a piece at a time since.

| Year | Change |
| --- | --- |
| 2011 | Desk web app (Java servlets on Tomcat, JSP), PostgreSQL schema, Perl nightly batch |
| 2012 | Title-level holds |
| 2016 | "Borrower" renamed "Patron" (CIRC-412); the `borrowers` view stays for the state report |
| 2019 | Fines job rewritten in Ruby around `sp_assess_fines`; loan periods moved to `config/loan-rules.json` |
| 2022 | Self-checkout kiosks with a gRPC gateway |
| 2024 | Moved to AWS: ECS Fargate, RDS, S3 for notice archives; Dockerfile and Terraform added |
| 2025 | Interlibrary loan intake moved to the Prairie Libraries Consortium |

<!-- @gloss documents circ:CircWebApp, circ:NightlyBatch, circ:KioskGateway conf=0.85 -->
## Pieces

- **Desk web app** (`src/main/java`, `src/main/webapp`): servlets mapped in `web.xml`, a thin service layer (`CirculationService`, `RenewalService`, `HoldQueue`, `FineCalculator`, `LoanPeriodCalculator`) and hand-written JDBC DAOs. No framework.
- **Database** (`db/`): numbered schema files and three stored procedures. The procedures hold the rules the batch needs inside a transaction.
- **Nightly batch** (`batch/`): `run_nightly.sh` runs fines, then notices, then hold expiry.
- **Kiosk gateway** (`kiosk/`): see [kiosk/README.md](../kiosk/README.md).
- **Infrastructure** (`infra/terraform`): one RDS instance, one ECS cluster with the web and kiosk services, and the nightly scheduled task.

<!-- @gloss documents lib:FineAssessment conf=0.85 -->
## The two fine calculations

Fines are computed in two places: `FineCalculator.java` at check-in, and `sp_assess_fines` every night for items still out. They implement the same rule (grace period, daily rate by format, juveniles exempt, seniors half, capped) and must change together. `FineCalculatorTest` covers the Java side; nothing tests the procedure.

## Configuration

| File | Read by | Holds |
| --- | --- | --- |
| `config/circulation.yaml` | web app, batch | renewals, grace period, daily rates, cap, block threshold, pickup days, closed days |
| `config/loan-rules.json` | web app | loan period and renewability per format |
| `config/patron-types.json` | web app | borrowing limit and fine multiplier per patron type |
| `config/branches.json` | web app, batch | branch codes and the mail run |
| `config/kiosk.json` | kiosk gateway | kiosk ids, branches, idle timeout |
| `config/enums.yaml` | web app, batch, schema | shared code tables |
