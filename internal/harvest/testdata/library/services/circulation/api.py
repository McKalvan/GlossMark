from fastapi import APIRouter

router = APIRouter(prefix="/loans")


@router.post("/{loan_id}/renew")
def renew(loan_id: int) -> None:
    raise HTTPException(status_code=409, detail="hold queue is not empty")


def mark_overdue(loan_id: int) -> None:
    store.update(loan_id, LoanPatch(status="overdue", flagged=True))
