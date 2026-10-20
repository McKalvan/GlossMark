/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

/** Mirrors LoanStatus in config/enums.yaml and the CHECK constraint on loans.status. */
// @gloss represents lib:LoanStatus conf=0.95
public enum LoanStatus {
    // @gloss represents lib:Active scope=line conf=0.8
    ACTIVE,
    // @gloss represents lib:LoanStatusOverdue scope=line conf=0.8
    OVERDUE,
    // @gloss represents lib:Returned scope=line conf=0.8
    RETURNED,
    // @gloss represents lib:Lost scope=line conf=0.8
    LOST;

    public boolean isOpen() {
        return this == ACTIVE || this == OVERDUE;
    }
}
