from collections.abc import AsyncIterator

import asyncpg
from fastapi import Request

from ordering.settings import get_settings


async def create_pool() -> asyncpg.Pool:
    return await asyncpg.create_pool(get_settings().database_url, min_size=2, max_size=10)


async def get_connection(request: Request) -> AsyncIterator[asyncpg.Connection]:
    pool: asyncpg.Pool = request.app.state.pool
    async with pool.acquire() as conn:
        yield conn
