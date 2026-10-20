/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.fail;

import org.junit.Test;

/** Runs against the circ_test schema (make test-db); see docs/ARCHITECTURE.md. */
// @gloss verifies .RenewalHandler conf=0.85
public class RenewalServiceTest extends DbTestCase {

    @Test
    public void refusesWhenAnotherPatronIsWaiting() throws Exception {
        fixture("loan_with_waiting_hold.sql");
        try {
            renewalService().renew("31234000456789");
            fail("expected ON_HOLD");
        } catch (CircException e) {
            assertEquals("ON_HOLD", e.getCode());
        }
    }

    @Test
    public void refusesAfterMaxRenewals() throws Exception {
        fixture("loan_renewed_twice.sql");
        try {
            renewalService().renew("31234000456789");
            fail("expected MAX_RENEWALS");
        } catch (CircException e) {
            assertEquals("MAX_RENEWALS", e.getCode());
        }
    }

    @Test
    public void magazinesAreNotRenewable() throws Exception {
        fixture("magazine_loan.sql");
        try {
            renewalService().renew("31234000999001");
            fail("expected NOT_RENEWABLE");
        } catch (CircException e) {
            assertEquals("NOT_RENEWABLE", e.getCode());
        }
    }
}
