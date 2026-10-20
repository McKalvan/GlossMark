"""Cataloging intake: turns a new acquisition into a shelf-ready record."""
from models import Book, Item


# @gloss implements .BookIntake
# @gloss produces lib:Book
def catalog_book(isbn: str, copies: int) -> Book:
    return Book(isbn=isbn, copies=copies)


# @gloss implements .BookIntake
# @gloss produces lib:DVD
def catalog_dvd(upc: str) -> Item:
    return Item(upc=upc)


# @gloss? produces lib:Laptop conf=0.55 note="tech-desk lending reuses the catalog"
def catalog_laptop(asset_tag: str) -> Item:
    return Item(asset_tag=asset_tag)
