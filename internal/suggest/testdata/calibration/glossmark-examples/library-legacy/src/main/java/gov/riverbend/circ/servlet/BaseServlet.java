/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.servlet;

import gov.riverbend.circ.service.CircException;
import gov.riverbend.circ.service.CirculationService;
import gov.riverbend.circ.service.HoldQueue;
import gov.riverbend.circ.service.RenewalService;

import java.io.IOException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

/** Shared plumbing: services from the ServletContext, staff branch, error page. */
public abstract class BaseServlet extends HttpServlet {

    private static final long serialVersionUID = 1L;

    protected CirculationService circulation() {
        return (CirculationService) getServletContext().getAttribute("circulation");
    }

    protected RenewalService renewals() {
        return (RenewalService) getServletContext().getAttribute("renewals");
    }

    protected HoldQueue holdQueue() {
        return (HoldQueue) getServletContext().getAttribute("holdQueue");
    }

    protected long staffBranch(HttpServletRequest req) {
        Object b = req.getSession().getAttribute("branchId");
        return b == null ? 1L : ((Long) b).longValue();
    }

    protected void refuse(HttpServletRequest req, HttpServletResponse resp, CircException e)
            throws ServletException, IOException {
        req.setAttribute("errorCode", e.getCode());
        req.setAttribute("errorMessage", e.getMessage());
        req.getRequestDispatcher("/WEB-INF/jsp/refused.jsp").forward(req, resp);
    }
}
