/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.dao;

import gov.riverbend.circ.model.Loan;
import gov.riverbend.circ.model.LoanStatus;

import java.sql.CallableStatement;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.sql.Timestamp;
import java.sql.Types;

public class LoanDAO {

    public long insert(Connection conn, Loan loan) throws SQLException {
        PreparedStatement ps = null;
        ResultSet keys = null;
        try {
            ps = conn.prepareStatement(
                "INSERT INTO loans (patron_id, item_id, branch_id, checked_out_at, due_date, status) "
                + "VALUES (?, ?, ?, ?, ?, ?)", Statement.RETURN_GENERATED_KEYS);
            ps.setLong(1, loan.getPatronId());
            ps.setLong(2, loan.getItemId());
            ps.setLong(3, loan.getBranchId());
            ps.setTimestamp(4, new Timestamp(loan.getCheckedOutAt().getTime()));
            ps.setDate(5, new java.sql.Date(loan.getDueDate().getTime()));
            ps.setString(6, loan.getStatus().name());
            ps.executeUpdate();
            keys = ps.getGeneratedKeys();
            keys.next();
            return keys.getLong(1);
        } finally {
            ConnectionFactory.closeQuietly(keys);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public Loan findOpenByItem(Connection conn, long itemId) throws SQLException {
        PreparedStatement ps = null;
        ResultSet rs = null;
        try {
            ps = conn.prepareStatement(
                "SELECT id, patron_id, item_id, branch_id, checked_out_at, due_date, renewals, status "
                + "FROM loans WHERE item_id = ? AND status IN ('ACTIVE','OVERDUE')");
            ps.setLong(1, itemId);
            rs = ps.executeQuery();
            if (!rs.next()) {
                return null;
            }
            Loan l = new Loan();
            l.setId(rs.getLong("id"));
            l.setPatronId(rs.getLong("patron_id"));
            l.setItemId(rs.getLong("item_id"));
            l.setBranchId(rs.getLong("branch_id"));
            l.setCheckedOutAt(rs.getTimestamp("checked_out_at"));
            l.setDueDate(rs.getDate("due_date"));
            l.setRenewals(rs.getInt("renewals"));
            l.setStatus(LoanStatus.valueOf(rs.getString("status")));
            return l;
        } finally {
            ConnectionFactory.closeQuietly(rs);
            ConnectionFactory.closeQuietly(ps);
        }
    }

    public void markReturned(Connection conn, long loanId, Timestamp at) throws SQLException {
        PreparedStatement ps = null;
        try {
            ps = conn.prepareStatement(
                "UPDATE loans SET returned_at = ?, status = 'RETURNED' WHERE id = ?");
            ps.setTimestamp(1, at);
            ps.setLong(2, loanId);
            ps.executeUpdate();
        } finally {
            ConnectionFactory.closeQuietly(ps);
        }
    }

    /** Calls sp_renew_loan, which re-checks holds and the renewal limit inside one transaction. */
    public java.sql.Date renew(Connection conn, long loanId, int periodDays) throws SQLException {
        CallableStatement cs = null;
        try {
            cs = conn.prepareCall("{ call sp_renew_loan(?, ?, ?) }");
            cs.setLong(1, loanId);
            cs.setInt(2, periodDays);
            cs.registerOutParameter(3, Types.DATE);
            cs.execute();
            return cs.getDate(3);
        } finally {
            ConnectionFactory.closeQuietly(cs);
        }
    }
}
