"""Finance exports of approved claims."""
import csv
import io

from flask import Blueprint, Response

from app import store
from app.models import ClaimStatus
from app.platform.auth import requires_role
from app.platform.flags import feature
from app.platform.ratelimit import rate_limit

bp = Blueprint("exports", __name__)


# @gloss implements .ApprovedClaimsExport
@bp.get("/approved.csv")
@feature("bulk_export")
@requires_role("finance")
@rate_limit("export")
def export_approved_csv():
    buf = io.StringIO()
    writer = csv.writer(buf)
    writer.writerow(["id", "claimant_id", "amount", "currency", "decided_by"])
    for claim in store.all_claims():
        if claim.status is ClaimStatus.APPROVED:
            writer.writerow([claim.id, claim.claimant_id, claim.amount, claim.currency, claim.decided_by])
    return Response(buf.getvalue(), mimetype="text/csv")
