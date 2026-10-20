/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

import java.util.Date;

/** One item checked out to one patron, from checkout until check-in. */
// @gloss represents lib:Loan conf=0.95
public class Loan {

    private long id;
    private long patronId;
    private long itemId;
    private long branchId;
    private Date checkedOutAt;
    private Date dueDate;
    private Date returnedAt;
    private int renewals;
    private LoanStatus status = LoanStatus.ACTIVE;

    public long getId() { return id; }
    public void setId(long id) { this.id = id; }

    public long getPatronId() { return patronId; }
    public void setPatronId(long patronId) { this.patronId = patronId; }

    public long getItemId() { return itemId; }
    public void setItemId(long itemId) { this.itemId = itemId; }

    public long getBranchId() { return branchId; }
    public void setBranchId(long branchId) { this.branchId = branchId; }

    public Date getCheckedOutAt() { return checkedOutAt; }
    public void setCheckedOutAt(Date checkedOutAt) { this.checkedOutAt = checkedOutAt; }

    public Date getDueDate() { return dueDate; }
    public void setDueDate(Date dueDate) { this.dueDate = dueDate; }

    public Date getReturnedAt() { return returnedAt; }
    public void setReturnedAt(Date returnedAt) { this.returnedAt = returnedAt; }

    public int getRenewals() { return renewals; }
    public void setRenewals(int renewals) { this.renewals = renewals; }

    public LoanStatus getStatus() { return status; }
    public void setStatus(LoanStatus status) { this.status = status; }
}
