from contextlib import asynccontextmanager

from fastapi import FastAPI

from ordering.db import create_pool
from ordering.routers import menu, orders, payments, tickets


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.pool = await create_pool()
    try:
        yield
    finally:
        await app.state.pool.close()


# @gloss implements .OrderingApi conf=0.9
def create_app() -> FastAPI:
    app = FastAPI(title="Pepper Row ordering", version="0.9.0", lifespan=lifespan)
    app.include_router(menu.router, prefix="/locations/{location_id}/menu", tags=["menu"])
    app.include_router(orders.router, prefix="/orders", tags=["orders"])
    app.include_router(payments.router, prefix="/payments", tags=["payments"])
    app.include_router(tickets.router, prefix="/tickets", tags=["tickets"])

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    return app


app = create_app()
