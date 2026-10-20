/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2013 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.servlet;

import gov.riverbend.circ.dao.ConnectionFactory;
import gov.riverbend.circ.dao.FineDAO;

import java.io.IOException;
import java.math.BigDecimal;
import java.sql.Connection;
import java.sql.SQLException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

/** POST /circ/fines/pay: cash or card payment taken at the desk. */
// @gloss implements lib:FinePayment conf=0.6 note="the glossary has no term for paying a fine at the desk"
public class FinePaymentServlet extends BaseServlet {

    private static final long serialVersionUID = 1L;

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse resp) throws ServletException, IOException {
        Connection conn = null;
        try {
            conn = ConnectionFactory.getConnection();
            long fineId = Long.parseLong(req.getParameter("fineId"));
            BigDecimal amount = new BigDecimal(req.getParameter("amount"));
            new FineDAO().recordPayment(conn, fineId, amount);
            resp.sendRedirect(req.getContextPath() + "/circ/patron?card=" + req.getParameter("card"));
        } catch (SQLException e) {
            throw new ServletException(e);
        } finally {
            ConnectionFactory.closeQuietly(conn);
        }
    }
}
