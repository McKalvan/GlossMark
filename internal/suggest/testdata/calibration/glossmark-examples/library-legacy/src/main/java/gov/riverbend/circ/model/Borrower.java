/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

/**
 * @deprecated since 3.2 (2016). Use {@link Patron}. Kept because
 *             BorrowerActivityReport and the state annual-report export still
 *             read the borrowers view, which has the pre-2016 column names.
 */
// @gloss represents lib:Borrower conf=0.9
@Deprecated
public class Borrower {

    private long borrowerId;
    private String barcode;
    private String name;
    private String borrowerClass;
    private int itemsOut;

    public long getBorrowerId() { return borrowerId; }
    public void setBorrowerId(long borrowerId) { this.borrowerId = borrowerId; }

    public String getBarcode() { return barcode; }
    public void setBarcode(String barcode) { this.barcode = barcode; }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }

    public String getBorrowerClass() { return borrowerClass; }
    public void setBorrowerClass(String borrowerClass) { this.borrowerClass = borrowerClass; }

    public int getItemsOut() { return itemsOut; }
    public void setItemsOut(int itemsOut) { this.itemsOut = itemsOut; }
}
