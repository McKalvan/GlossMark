/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.dao;

import gov.riverbend.circ.model.Item;
import gov.riverbend.circ.model.ItemFormat;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

public class ItemDAO {

    public Item findByBarcode(Connection conn, String barcode) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(
                "SELECT id, barcode, title_id, branch_id, format, call_number, replacement_cost, is_reference "
                + "FROM items WHERE barcode = ?");
            ps.setString(1, barcode);
            rs = ps.executeQuery();
            if (!rs.next()) {
                return null;
            }
            Item i = new Item();
            i.setId(rs.getLong("id"));
            i.setBarcode(rs.getString("barcode"));
            i.setTitleId(rs.getLong("title_id"));
            i.setBranchId(rs.getLong("branch_id"));
            i.setFormat(ItemFormat.fromCode(rs.getString("format")));
            i.setCallNumber(rs.getString("call_number"));
            i.setReplacementCost(rs.getBigDecimal("replacement_cost"));
            i.setReference(rs.getBoolean("is_reference"));
            return i;
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public boolean isCheckedOut(Connection conn, long itemId) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(
                "SELECT 1 FROM loans WHERE item_id = ? AND status IN ('ACTIVE','OVERDUE')");
            ps.setLong(1, itemId);
            rs = ps.executeQuery();
            return rs.next();
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }
}
