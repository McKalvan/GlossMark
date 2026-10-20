/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.dao;

import gov.riverbend.circ.model.Fine;

import java.math.BigDecimal;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.SQLException;

public class FineDAO {

    public void insert(Connection conn, Fine fine) throws SQLException {
        PreparedStatement ps = null;
        try {
            ps = conn.prepareStatement(
                "INSERT INTO fines (patron_id, loan_id, fine_type, amount, assessed_at) "
                + "VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)");
            ps.setLong(1, fine.getPatronId());
            if (fine.getLoanId() == null) {
                ps.setNull(2, java.sql.Types.BIGINT);
            } else {
                ps.setLong(2, fine.getLoanId());
            }
            ps.setString(3, fine.getFineType());
            ps.setBigDecimal(4, fine.getAmount());
            ps.executeUpdate();
        } finally {
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public void recordPayment(Connection conn, long fineId, BigDecimal amount) throws SQLException {
        PreparedStatement ps = null;
        try {
            ps = conn.prepareStatement("UPDATE fines SET paid = paid + ? WHERE id = ?");
            ps.setBigDecimal(1, amount);
            ps.setLong(2, fineId);
            ps.executeUpdate();
            ps.close();
            ps = conn.prepareStatement(
                "UPDATE patrons SET balance_owed = balance_owed - ? "
                + "WHERE id = (SELECT patron_id FROM fines WHERE id = ?)");
            ps.setBigDecimal(1, amount);
            ps.setLong(2, fineId);
            ps.executeUpdate();
        } finally {
            ConnectionFactory.closeQuietly(ps);
        }
    }
}
