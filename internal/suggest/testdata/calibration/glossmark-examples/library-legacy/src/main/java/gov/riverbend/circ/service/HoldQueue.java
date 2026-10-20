/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2012 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import gov.riverbend.circ.dao.HoldDAO;
import gov.riverbend.circ.model.Hold;
import gov.riverbend.circ.model.Item;
import gov.riverbend.circ.util.DateUtil;

import java.sql.Connection;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.util.Date;

/**
 * Title-level holds, first come first served. When a copy of a wanted title
 * is checked in, the oldest waiting hold traps it and the desk prints a slip.
 */
// @gloss implements .HoldQueue conf=0.95
public class HoldQueue {

    private final HoldDAO holds = new HoldDAO();
    private final CircConfig config;

    public HoldQueue(CircConfig config) {
        this.config = config;
    }

    public long place(Connection conn, long patronId, long titleId, long pickupBranchId) throws SQLException {
        return holds.place(conn, patronId, titleId, pickupBranchId);
    }

    /** Returns the hold the item was trapped for, or null when nobody is waiting. */
    public Hold trapIfWanted(Connection conn, Item item, long branchId) throws SQLException {
        Hold next = holds.nextWaiting(conn, item.getTitleId());
        if (next == null) {
            return null;
        }
        Date pickupBy = DateUtil.addDays(new Date(), config.holdPickupDays());
        holds.trap(conn, next.getId(), item.getId(), new Timestamp(pickupBy.getTime()));
        next.setState(Hold.TRAPPED);
        next.setTrappedItemId(item.getId());
        return next;
    }
}
