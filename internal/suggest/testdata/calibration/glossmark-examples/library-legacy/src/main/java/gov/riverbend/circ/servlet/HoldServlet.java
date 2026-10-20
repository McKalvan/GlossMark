/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2012 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.servlet;

import gov.riverbend.circ.dao.ConnectionFactory;
import gov.riverbend.circ.dao.PatronDAO;
import gov.riverbend.circ.model.Patron;

import java.io.IOException;
import java.sql.Connection;
import java.sql.SQLException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

/** POST /circ/holds: place a title-level hold for pickup at a branch. */
public class HoldServlet extends BaseServlet {

    private static final long serialVersionUID = 1L;

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse resp) throws ServletException, IOException {
        Connection conn = null;
        try {
            conn = ConnectionFactory.getConnection();
            Patron patron = new PatronDAO().findByCard(conn, req.getParameter("card"));
            if (patron == null) {
                resp.sendError(HttpServletResponse.SC_NOT_FOUND, "card not found");
                return;
            }
            long titleId = Long.parseLong(req.getParameter("titleId"));
            long pickup = Long.parseLong(req.getParameter("pickupBranch"));
            long holdId = holdQueue().place(conn, patron.getId(), titleId, pickup);
            req.setAttribute("holdId", Long.valueOf(holdId));
            req.getRequestDispatcher("/WEB-INF/jsp/hold-placed.jsp").forward(req, resp);
        } catch (SQLException e) {
            throw new ServletException(e);
        } finally {
            ConnectionFactory.closeQuietly(conn);
        }
    }
}
