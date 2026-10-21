# @gloss verifies .BearerAuthentication scope=file
def test_missing_token_is_rejected(app):
    assert app.test_client().get("/claims").status_code == 401


def test_bad_token_is_rejected(app):
    resp = app.test_client().get("/claims", headers={"Authorization": "Bearer nope"})
    assert resp.status_code == 401


def test_health_is_public(app):
    assert app.test_client().get("/healthz").status_code == 200


def test_approve_requires_approver_role(app, token):
    resp = app.test_client().post(
        "/claims/00000000-0000-0000-0000-000000000000/approve",
        headers={"Authorization": f"Bearer {token(roles=['employee'])}"},
    )
    assert resp.status_code == 403
