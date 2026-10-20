/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.servlet;

import gov.riverbend.circ.model.Loan;
import gov.riverbend.circ.service.CircException;

import java.io.IOException;
import java.sql.SQLException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

/** POST /circ/checkin: barcode from the desk scanner or the book drop. */
// @gloss implements .CheckinHandler conf=0.75
public class CheckinServlet extends BaseServlet {

    private static final long serialVersionUID = 1L;

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse resp) throws ServletException, IOException {
        try {
            Loan loan = circulation().checkin(req.getParameter("barcode"), staffBranch(req));
            req.setAttribute("loan", loan);
            req.getRequestDispatcher("/WEB-INF/jsp/checkin.jsp").forward(req, resp);
        } catch (CircException e) {
            refuse(req, resp, e);
        } catch (SQLException e) {
            throw new ServletException(e);
        }
    }
}
