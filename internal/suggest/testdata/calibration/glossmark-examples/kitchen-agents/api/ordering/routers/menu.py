from uuid import UUID

import asyncpg
from fastapi import APIRouter, Depends, status

from ordering.db import get_connection
from ordering.models import MenuSection
from ordering.repositories.menu_repo import MenuRepository
from ordering.services.menu_catalog import MenuCatalog

router = APIRouter()


def catalog(conn: asyncpg.Connection = Depends(get_connection)) -> MenuCatalog:
    return MenuCatalog(MenuRepository(conn))


# @gloss? implements .MenuCatalog conf=0.85 id=menu-get-route note="the only reader of the published menu"
@router.get("", response_model=list[MenuSection])
async def get_menu(location_id: UUID, svc: MenuCatalog = Depends(catalog)) -> list[MenuSection]:
    return await svc.published_menu(location_id)


@router.post("/items/{item_id}/86", status_code=status.HTTP_204_NO_CONTENT)
async def eighty_six_item(location_id: UUID, item_id: UUID, svc: MenuCatalog = Depends(catalog)) -> None:
    await svc.mark_unavailable(item_id)


@router.delete("/items/{item_id}/86", status_code=status.HTTP_204_NO_CONTENT)
async def restore_item(location_id: UUID, item_id: UUID, svc: MenuCatalog = Depends(catalog)) -> None:
    await svc.restore(item_id)
