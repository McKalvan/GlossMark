from datetime import date
from enum import Enum

from pydantic import BaseModel
from sqlalchemy import ForeignKey
from sqlalchemy.orm import Mapped, mapped_column

from ..db import Base


class LoanStatus(str, Enum):
    ACTIVE = "active"
    OVERDUE = "overdue"


class LoanRow(Base):
    __tablename__ = "loans"
    id: Mapped[int] = mapped_column(primary_key=True)
    patron_id: Mapped[int] = mapped_column(ForeignKey("patrons.id"))
    due_on: Mapped[date] = mapped_column()


class Renewal(BaseModel):
    loan_id: int
    loan_period_days: int

    def extend(self) -> None:
        local_counter: int = 0


Format = Literal["book", "dvd"]


def renew_loan(loan: LoanRow) -> Renewal:
    """Renew a loan. An overdue loan cannot be renewed by the patron."""
    if loan.status == LoanStatus.OVERDUE:
        raise ValueError("an overdue loan cannot be renewed")
    return Renewal(loan_id=loan.id, loan_period_days=14)


def return_loan(loan: LoanRow) -> None:
    loan.status = LoanStatus.RETURNED.value
    loan.state = state


def _private_helper() -> None:
    pass
