/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.servlet;

import gov.riverbend.circ.service.CircException;

import java.io.IOException;
import java.sql.SQLException;
import java.util.Date;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

/** POST /circ/renew: from the desk and from the OPAC account page. */
// @gloss implements .RenewalHandler conf=0.75
public class RenewalServlet extends BaseServlet {

    private static final long serialVersionUID = 1L;

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse resp) throws ServletException, IOException {
        try {
            Date due = renewals().renew(req.getParameter("barcode"));
            req.setAttribute("newDueDate", due);
            req.getRequestDispatcher("/WEB-INF/jsp/renewed.jsp").forward(req, resp);
        } catch (CircException e) {
            refuse(req, resp, e);
        } catch (SQLException e) {
            throw new ServletException(e);
        }
    }
}
