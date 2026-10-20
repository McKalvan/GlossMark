/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import gov.riverbend.circ.dao.ConnectionFactory;
import gov.riverbend.circ.dao.HoldDAO;
import gov.riverbend.circ.dao.ItemDAO;
import gov.riverbend.circ.dao.LoanDAO;
import gov.riverbend.circ.model.Item;
import gov.riverbend.circ.model.Loan;

import java.sql.Connection;
import java.sql.SQLException;
import java.util.Date;

/** Renewals from the desk, the OPAC "My account" page and the kiosks. */
// @gloss implements .RenewalHandler conf=0.85
public class RenewalService {

    private final ItemDAO items = new ItemDAO();
    private final LoanDAO loans = new LoanDAO();
    private final HoldDAO holds = new HoldDAO();
    private final LoanPeriodCalculator periods;
    private final CircConfig config;

    public RenewalService(LoanPeriodCalculator periods, CircConfig config) {
        this.periods = periods;
        this.config = config;
    }

    public Date renew(String itemBarcode) throws CircException, SQLException {
        Connection conn = ConnectionFactory.getConnection();
        try {
            Item item = items.findByBarcode(conn, itemBarcode);
            Loan loan = item == null ? null : loans.findOpenByItem(conn, item.getId());
            if (loan == null) {
                throw new CircException("NOT_OUT", "Item is not checked out");
            }
            if (!periods.isRenewable(item)) {
                throw new CircException("NOT_RENEWABLE", item.getFormat() + " items cannot be renewed");
            }
            if (loan.getRenewals() >= config.maxRenewals()) {
                throw new CircException("MAX_RENEWALS", "Renewed " + loan.getRenewals() + " times already");
            }
            if (holds.hasWaiting(conn, item.getTitleId())) {
                throw new CircException("ON_HOLD", "Another patron is waiting for this title");
            }
            return loans.renew(conn, loan.getId(), periods.periodDays(item));
        } finally {
            ConnectionFactory.closeQuietly(conn);
        }
    }
}
