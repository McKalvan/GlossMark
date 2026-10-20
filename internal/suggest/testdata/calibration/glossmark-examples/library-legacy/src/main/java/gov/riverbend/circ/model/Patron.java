/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

import java.math.BigDecimal;
import java.util.Date;

/**
 * A registered library user. Replaced the old Borrower class in 2016 (CIRC-412);
 * the borrowers view and BorrowerActivityReport still use the old name.
 */
// @gloss represents lib:Patron conf=0.95
public class Patron {

    private long id;
    private String cardNumber;
    private String lastName;
    private String firstName;
    private String email;
    private String phone;
    private PatronType type;
    private long homeBranchId;
    private Date expiresOn;
    private BigDecimal balanceOwed = BigDecimal.ZERO;
    private boolean emailNotices;

    public enum PatronType { ADULT, JUVENILE, SENIOR, NONRESIDENT }

    public long getId() { return id; }
    public void setId(long id) { this.id = id; }

    public String getCardNumber() { return cardNumber; }
    public void setCardNumber(String cardNumber) { this.cardNumber = cardNumber; }

    public String getLastName() { return lastName; }
    public void setLastName(String lastName) { this.lastName = lastName; }

    public String getFirstName() { return firstName; }
    public void setFirstName(String firstName) { this.firstName = firstName; }

    public String getEmail() { return email; }
    public void setEmail(String email) { this.email = email; }

    public String getPhone() { return phone; }
    public void setPhone(String phone) { this.phone = phone; }

    public PatronType getType() { return type; }
    public void setType(PatronType type) { this.type = type; }

    public long getHomeBranchId() { return homeBranchId; }
    public void setHomeBranchId(long homeBranchId) { this.homeBranchId = homeBranchId; }

    public Date getExpiresOn() { return expiresOn; }
    public void setExpiresOn(Date expiresOn) { this.expiresOn = expiresOn; }

    public BigDecimal getBalanceOwed() { return balanceOwed; }
    public void setBalanceOwed(BigDecimal balanceOwed) { this.balanceOwed = balanceOwed; }

    public boolean isEmailNotices() { return emailNotices; }
    public void setEmailNotices(boolean emailNotices) { this.emailNotices = emailNotices; }

    public boolean isExpired(Date today) {
        return expiresOn != null && expiresOn.before(today);
    }

    public String getDisplayName() {
        return lastName + ", " + firstName;
    }
}
