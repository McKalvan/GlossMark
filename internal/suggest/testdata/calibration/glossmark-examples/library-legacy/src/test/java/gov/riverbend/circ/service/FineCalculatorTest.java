/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import static org.junit.Assert.assertEquals;

import gov.riverbend.circ.model.Item;
import gov.riverbend.circ.model.ItemFormat;
import gov.riverbend.circ.model.Loan;
import gov.riverbend.circ.model.Patron;
import gov.riverbend.circ.util.DateUtil;

import java.math.BigDecimal;
import java.util.Date;

import org.junit.Before;
import org.junit.Test;

// @gloss tests .DeskFineCalculator, .FineCapRule, .GracePeriodRule conf=0.85
public class FineCalculatorTest {

    private FineCalculator calc;
    private Item dvd;
    private Loan loan;
    private Patron adult;

    @Before
    public void setUp() throws Exception {
        calc = new FineCalculator(new CircConfig("src/test/resources/circulation.yaml"));
        dvd = new Item();
        dvd.setFormat(ItemFormat.DVD);
        dvd.setReplacementCost(new BigDecimal("24.99"));
        loan = new Loan();
        loan.setDueDate(DateUtil.addDays(new Date(), -10));
        adult = new Patron();
        adult.setType(Patron.PatronType.ADULT);
    }

    @Test
    public void nothingWithinGracePeriod() {
        loan.setDueDate(DateUtil.addDays(new Date(), -2));
        assertEquals(BigDecimal.ZERO, calc.overdueFine(loan, dvd, adult, new Date()));
    }

    @Test
    public void dvdAccruesOneDollarPerDayAfterGrace() {
        assertEquals(new BigDecimal("7.00"), calc.overdueFine(loan, dvd, adult, new Date()));
    }

    @Test
    public void juvenilesPayNoOverdueFines() {
        Patron kid = new Patron();
        kid.setType(Patron.PatronType.JUVENILE);
        assertEquals(BigDecimal.ZERO, calc.overdueFine(loan, dvd, kid, new Date()));
    }

    @Test
    public void capIsTheLowerOfReplacementCostAndFlatCap() {
        Item cheap = new Item();
        cheap.setReplacementCost(new BigDecimal("6.50"));
        assertEquals(new BigDecimal("6.50"), calc.applyCap(new BigDecimal("40.00"), cheap));
        assertEquals(new BigDecimal("20.00"), calc.applyCap(new BigDecimal("40.00"), dvd));
    }
}
