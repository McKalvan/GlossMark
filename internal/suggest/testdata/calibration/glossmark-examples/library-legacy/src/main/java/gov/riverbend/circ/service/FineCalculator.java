/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import gov.riverbend.circ.model.Item;
import gov.riverbend.circ.model.Loan;
import gov.riverbend.circ.model.Patron;
import gov.riverbend.circ.util.DateUtil;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.util.Date;

/**
 * Overdue fines assessed at check-in. The nightly job (batch/assess_fines.rb,
 * sp_assess_fines) accrues the same amounts for items still out; both must
 * agree or patrons see their balance jump at the desk.
 */
// @gloss implements .DeskFineCalculator conf=0.82
public class FineCalculator {

    private final CircConfig config;

    public FineCalculator(CircConfig config) {
        this.config = config;
    }

    public BigDecimal overdueFine(Loan loan, Item item, Patron patron, Date returnedAt) {
        // @gloss implements .GracePeriodRule scope=line conf=0.8
        int late = DateUtil.daysBetween(loan.getDueDate(), returnedAt) - config.graceDays();
        if (late <= 0 || patron.getType() == Patron.PatronType.JUVENILE) {
            return BigDecimal.ZERO;
        }
        BigDecimal fine = config.dailyRate(item.getFormat().name()).multiply(BigDecimal.valueOf(late));
        if (patron.getType() == Patron.PatronType.SENIOR) {
            fine = fine.divide(BigDecimal.valueOf(2), 2, RoundingMode.HALF_UP);
        }
        return applyCap(fine, item);
    }

    // @gloss implements .FineCapRule conf=0.85
    /** A fine never exceeds the item's replacement cost, nor the flat cap. */
    public BigDecimal applyCap(BigDecimal fine, Item item) {
        BigDecimal cap = config.fineCapFlat();
        if (item.getReplacementCost() != null && item.getReplacementCost().compareTo(cap) < 0) {
            cap = item.getReplacementCost();
        }
        return fine.min(cap);
    }
}
