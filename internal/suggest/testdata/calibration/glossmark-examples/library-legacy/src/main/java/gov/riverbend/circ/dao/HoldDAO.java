/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2012 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.dao;

import gov.riverbend.circ.model.Hold;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;

public class HoldDAO {

    public long place(Connection conn, long patronId, long titleId, long pickupBranchId) throws SQLException {
        PreparedStatement ps = null;
        ResultSet keys = null;
        try {
            ps = conn.prepareStatement(
                "INSERT INTO holds (patron_id, title_id, pickup_branch_id, placed_at, state) "
                + "VALUES (?, ?, ?, CURRENT_TIMESTAMP, 'WAITING')", PreparedStatement.RETURN_GENERATED_KEYS);
            ps.setLong(1, patronId);
            ps.setLong(2, titleId);
            ps.setLong(3, pickupBranchId);
            ps.executeUpdate();
            keys = ps.getGeneratedKeys();
            keys.next();
            return keys.getLong(1);
        } finally {
            ConnectionFactory.closeQuietly(keys);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    /** Oldest waiting hold on the title, first come first served. */
    public Hold nextWaiting(Connection conn, long titleId) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(
                "SELECT id, patron_id, title_id, pickup_branch_id, placed_at FROM holds "
                + "WHERE title_id = ? AND state = 'WAITING' ORDER BY placed_at, id FOR UPDATE");
            ps.setLong(1, titleId);
            ps.setMaxRows(1);
            rs = ps.executeQuery();
            if (!rs.next()) {
                return null;
            }
            Hold h = new Hold();
            h.setId(rs.getLong("id"));
            h.setPatronId(rs.getLong("patron_id"));
            h.setTitleId(rs.getLong("title_id"));
            h.setPickupBranchId(rs.getLong("pickup_branch_id"));
            h.setPlacedAt(rs.getTimestamp("placed_at"));
            return h;
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public void trap(Connection conn, long holdId, long itemId, Timestamp pickupBy) throws SQLException {
        PreparedStatement ps = null;
        try {
            ps = conn.prepareStatement(
                "UPDATE holds SET state = 'TRAPPED', trapped_item_id = ?, trapped_at = CURRENT_TIMESTAMP, "
                + "pickup_by = ? WHERE id = ?");
            ps.setLong(1, itemId);
            ps.setTimestamp(2, pickupBy);
            ps.setLong(3, holdId);
            ps.executeUpdate();
        } finally {
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public boolean hasWaiting(Connection conn, long titleId) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement("SELECT 1 FROM holds WHERE title_id = ? AND state = 'WAITING'");
            ps.setLong(1, titleId);
            rs = ps.executeQuery();
            return rs.next();
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }
}
