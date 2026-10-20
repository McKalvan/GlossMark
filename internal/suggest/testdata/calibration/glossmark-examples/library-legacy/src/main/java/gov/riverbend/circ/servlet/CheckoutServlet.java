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

/** POST /circ/checkout: card + barcode from the desk scanner. Mapped in web.xml. */
// @gloss implements .CheckoutHandler conf=0.75
public class CheckoutServlet extends BaseServlet {

    private static final long serialVersionUID = 1L;

    @Override
    protected void doPost(HttpServletRequest req, HttpServletResponse resp) throws ServletException, IOException {
        String card = req.getParameter("card");
        String barcode = req.getParameter("barcode");
        try {
            Loan loan = circulation().checkout(card, barcode, staffBranch(req));
            req.setAttribute("loan", loan);
            req.getRequestDispatcher("/WEB-INF/jsp/checkout.jsp").forward(req, resp);
        } catch (CircException e) {
            refuse(req, resp, e);
        } catch (SQLException e) {
            throw new ServletException(e);
        }
    }
}
