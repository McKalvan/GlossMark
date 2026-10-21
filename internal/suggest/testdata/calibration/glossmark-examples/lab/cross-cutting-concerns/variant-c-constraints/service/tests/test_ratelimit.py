# @gloss verifies .RateLimiting scope=file
def test_submission_burst_is_limited(app, token):
    client = app.test_client()
    headers = {"Authorization": f"Bearer {token(sub='emp-9')}"}
    codes = [client.post("/claims", json={"amount": "1"}, headers=headers).status_code for _ in range(12)]
    assert codes[:10] == [201] * 10
    assert 429 in codes[10:]
