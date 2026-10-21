def _auth(token, **kw):
    return {"Authorization": f"Bearer {token(**kw)}"}


# @gloss verifies .SubmitClaimHandler, .ClaimDecisionHandler
def test_submit_and_approve(app, token):
    client = app.test_client()
    created = client.post("/claims", json={"amount": "42.50"}, headers=_auth(token, sub="emp-1"))
    assert created.status_code == 201
    claim_id = created.get_json()["id"]
    approved = client.post(
        f"/claims/{claim_id}/approve", headers=_auth(token, sub="mgr-1", roles=["approver"])
    )
    assert approved.get_json()["status"] == "approved"


# @gloss verifies .ClaimDecisionHandler
def test_approver_cannot_approve_own_claim(app, token):
    client = app.test_client()
    created = client.post("/claims", json={"amount": "10"}, headers=_auth(token, sub="mgr-1"))
    resp = client.post(
        f"/claims/{created.get_json()['id']}/approve",
        headers=_auth(token, sub="mgr-1", roles=["approver"]),
    )
    assert resp.status_code == 403
