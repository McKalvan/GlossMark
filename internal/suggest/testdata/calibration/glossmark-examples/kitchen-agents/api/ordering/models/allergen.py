from enum import StrEnum

from pydantic import BaseModel, Field


# @gloss represents dine:Allergen conf=0.95
class Allergen(StrEnum):
    """The fourteen allergens UK food law requires a restaurant to declare."""

    CELERY = "celery"
    GLUTEN = "gluten"
    CRUSTACEANS = "crustaceans"
    EGGS = "eggs"
    FISH = "fish"
    LUPIN = "lupin"
    MILK = "milk"
    MOLLUSCS = "molluscs"
    MUSTARD = "mustard"
    TREE_NUTS = "tree_nuts"
    PEANUTS = "peanuts"
    SESAME = "sesame"
    SOYA = "soya"
    SULPHITES = "sulphites"


class AllergenDeclaration(BaseModel):
    """What a guest told the server, recorded on the order before it is fired."""

    allergens: frozenset[Allergen] = Field(default_factory=frozenset)
    acknowledged_by: str | None = None
    note: str | None = Field(default=None, max_length=280)

    @property
    def declared(self) -> bool:
        return bool(self.allergens)
