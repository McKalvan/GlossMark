/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2012 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import gov.riverbend.circ.model.Hold;
import gov.riverbend.circ.model.Item;

import org.junit.Test;

// @gloss verifies .HoldQueue conf=0.55
public class HoldQueueTest extends DbTestCase {

    @Test
    public void oldestWaitingHoldTrapsTheReturnedCopy() throws Exception {
        fixture("two_holds_same_title.sql");
        Item copy = new Item();
        copy.setId(501L);
        copy.setTitleId(77L);
        Hold trapped = holdQueue().trapIfWanted(conn, copy, 1L);
        assertEquals(9001L, trapped.getId());
        assertEquals(Hold.TRAPPED, trapped.getState());
    }

    @Test
    public void nobodyWaitingTrapsNothing() throws Exception {
        fixture("no_holds.sql");
        Item copy = new Item();
        copy.setId(502L);
        copy.setTitleId(78L);
        assertNull(holdQueue().trapIfWanted(conn, copy, 1L));
    }
}
