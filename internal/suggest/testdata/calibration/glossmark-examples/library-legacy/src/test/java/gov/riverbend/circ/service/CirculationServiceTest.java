/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.fail;

import gov.riverbend.circ.model.Patron;

import java.math.BigDecimal;
import java.util.Calendar;
import java.util.Date;

import org.junit.Before;
import org.junit.Test;

// @gloss verifies .PatronBlockCheck conf=0.85
public class CirculationServiceTest {

    private CirculationService service;
    private Patron patron;

    @Before
    public void setUp() throws Exception {
        CircConfig config = new CircConfig("src/test/resources/circulation.yaml");
        service = new CirculationService(config, null, null, null, null);
        patron = new Patron();
        patron.setType(Patron.PatronType.ADULT);
        patron.setBalanceOwed(BigDecimal.ZERO);
        Calendar c = Calendar.getInstance();
        c.add(Calendar.YEAR, 1);
        patron.setExpiresOn(c.getTime());
    }

    @Test
    public void patronUnderThresholdMayBorrow() throws Exception {
        patron.setBalanceOwed(new BigDecimal("9.99"));
        service.assertNotBlocked(patron, new Date());
    }

    @Test
    public void patronAtThresholdIsBlocked() {
        patron.setBalanceOwed(new BigDecimal("10.00"));
        try {
            service.assertNotBlocked(patron, new Date());
            fail("expected BLOCKED");
        } catch (CircException e) {
            assertEquals("BLOCKED", e.getCode());
        }
    }

    @Test
    public void expiredCardIsBlocked() {
        patron.setExpiresOn(new Date(0));
        try {
            service.assertNotBlocked(patron, new Date());
            fail("expected EXPIRED");
        } catch (CircException e) {
            assertEquals("EXPIRED", e.getCode());
        }
    }
}
