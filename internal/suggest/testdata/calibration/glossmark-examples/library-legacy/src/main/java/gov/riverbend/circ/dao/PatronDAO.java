/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.dao;

import gov.riverbend.circ.model.Patron;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

public class PatronDAO {

    private static final String SELECT =
        "SELECT id, card_number, last_name, first_name, email, phone, patron_type, "
        + "home_branch_id, expires_on, balance_owed, email_notices FROM patrons ";

    public Patron findByCard(Connection conn, String cardNumber) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(SELECT + "WHERE card_number = ?");
            ps.setString(1, cardNumber);
            rs = ps.executeQuery();
            return rs.next() ? map(rs) : null;
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public Patron findById(Connection conn, long id) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(SELECT + "WHERE id = ?");
            ps.setLong(1, id);
            rs = ps.executeQuery();
            return rs.next() ? map(rs) : null;
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public int countOpenLoans(Connection conn, long patronId) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(
                "SELECT COUNT(*) FROM loans WHERE patron_id = ? AND status IN ('ACTIVE','OVERDUE')");
            ps.setLong(1, patronId);
            rs = ps.executeQuery();
            rs.next();
            return rs.getInt(1);
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    private Patron map(ResultSet rs) throws SQLException {
        Patron p = new Patron();
        p.setId(rs.getLong("id"));
        p.setCardNumber(rs.getString("card_number"));
        p.setLastName(rs.getString("last_name"));
        p.setFirstName(rs.getString("first_name"));
        p.setEmail(rs.getString("email"));
        p.setPhone(rs.getString("phone"));
        p.setType(Patron.PatronType.valueOf(rs.getString("patron_type")));
        p.setHomeBranchId(rs.getLong("home_branch_id"));
        p.setExpiresOn(rs.getDate("expires_on"));
        p.setBalanceOwed(rs.getBigDecimal("balance_owed"));
        p.setEmailNotices(rs.getBoolean("email_notices"));
        return p;
    }
}
