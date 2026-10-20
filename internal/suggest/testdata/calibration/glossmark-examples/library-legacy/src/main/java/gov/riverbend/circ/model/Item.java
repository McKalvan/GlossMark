/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

import java.math.BigDecimal;

/** One physical copy, identified by the barcode on its spine label. */
// @gloss represents lib:Item conf=0.95
public class Item {

    private long id;
    private String barcode;
    private long titleId;
    private long branchId;
    private ItemFormat format;
    private String callNumber;
    private BigDecimal replacementCost;
    private boolean reference;

    public long getId() { return id; }
    public void setId(long id) { this.id = id; }

    public String getBarcode() { return barcode; }
    public void setBarcode(String barcode) { this.barcode = barcode; }

    public long getTitleId() { return titleId; }
    public void setTitleId(long titleId) { this.titleId = titleId; }

    public long getBranchId() { return branchId; }
    public void setBranchId(long branchId) { this.branchId = branchId; }

    public ItemFormat getFormat() { return format; }
    public void setFormat(ItemFormat format) { this.format = format; }

    public String getCallNumber() { return callNumber; }
    public void setCallNumber(String callNumber) { this.callNumber = callNumber; }

    public BigDecimal getReplacementCost() { return replacementCost; }
    public void setReplacementCost(BigDecimal replacementCost) { this.replacementCost = replacementCost; }

    public boolean isReference() { return reference; }
    public void setReference(boolean reference) { this.reference = reference; }
}
