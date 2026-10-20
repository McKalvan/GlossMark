from fastapi.testclient import TestClient

from ordering.main import create_app


def test_healthz_does_not_need_a_database():
    # Without a `with` block TestClient skips the lifespan, so no pool is opened.
    client = TestClient(create_app())
    assert client.get("/healthz").json() == {"status": "ok"}


# @gloss verifies .OrderingApi conf=0.9
def test_fire_route_is_registered():
    paths = {route.path for route in create_app().routes}
    assert "/orders/{order_id}/fire" in paths
    assert "/locations/{location_id}/menu/items/{item_id}/86" in paths
