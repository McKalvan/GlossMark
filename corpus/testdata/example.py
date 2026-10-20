"""Cataloging engine.

The word @gloss in a docstring is not a marker because it is not in a comment.
"""
from lib.models import Media, Book, DVD

# @gloss implements circ:BookIntake
def catalog_book(isbn: str, copies: int) -> Book:
    return Book(isbn, copies)


# @gloss? represents lib:DVD conf=0.72 note="agent guess from field names"
class DVDRecord:
    release_date: str


# @gloss produces lib:Book, lib:ItemReturned
@retry(times=3)
def return_item(item: DVD):
    s = "@gloss implements circ:Nothing"  # a string literal, never a marker
    return s


# @gloss? implements circ:NEW:DamageCheck note="no concept exists yet"
def check_damage(item: Media):
    pass
