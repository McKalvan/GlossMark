/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.dao;

import gov.riverbend.circ.model.Borrower;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;

/**
 * Reads the legacy borrowers view (db/schema/002_patrons.sql). Only
 * BorrowerActivityReport uses this; new code goes through PatronDAO.
 */
@Deprecated
public class BorrowerDAO {

    public List<Borrower> findActiveSince(Connection conn, java.sql.Date since) throws SQLException {
        List<Borrower> out = new ArrayList<Borrower>();
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(
                "SELECT borrower_id, barcode, borrower_name, borrower_class, items_out "
                + "FROM borrowers WHERE last_activity >= ? ORDER BY borrower_name");
            ps.setDate(1, since);
            rs = ps.executeQuery();
            while (rs.next()) {
                Borrower b = new Borrower();
                b.setBorrowerId(rs.getLong("borrower_id"));
                b.setBarcode(rs.getString("barcode"));
                b.setName(rs.getString("borrower_name"));
                b.setBorrowerClass(rs.getString("borrower_class"));
                b.setItemsOut(rs.getInt("items_out"));
                out.add(b);
            }
            return out;
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }
}
