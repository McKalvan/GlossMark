/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

import java.math.BigDecimal;
import java.util.Date;

/** Money a patron owes the library for one loan. */
// @gloss represents lib:Fine conf=0.95
public class Fine {

    private long id;
    private long patronId;
    private Long loanId;
    private String fineType;
    private BigDecimal amount;
    private BigDecimal paid = BigDecimal.ZERO;
    private Date assessedAt;
    private boolean waived;

    public long getId() { return id; }
    public void setId(long id) { this.id = id; }

    public long getPatronId() { return patronId; }
    public void setPatronId(long patronId) { this.patronId = patronId; }

    public Long getLoanId() { return loanId; }
    public void setLoanId(Long loanId) { this.loanId = loanId; }

    public String getFineType() { return fineType; }
    public void setFineType(String fineType) { this.fineType = fineType; }

    public BigDecimal getAmount() { return amount; }
    public void setAmount(BigDecimal amount) { this.amount = amount; }

    public BigDecimal getPaid() { return paid; }
    public void setPaid(BigDecimal paid) { this.paid = paid; }

    public Date getAssessedAt() { return assessedAt; }
    public void setAssessedAt(Date assessedAt) { this.assessedAt = assessedAt; }

    public boolean isWaived() { return waived; }
    public void setWaived(boolean waived) { this.waived = waived; }

    public BigDecimal getOutstanding() {
        return waived ? BigDecimal.ZERO : amount.subtract(paid);
    }
}
