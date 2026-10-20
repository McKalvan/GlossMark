/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import gov.riverbend.circ.dao.ConnectionFactory;
import gov.riverbend.circ.dao.FineDAO;
import gov.riverbend.circ.dao.ItemDAO;
import gov.riverbend.circ.dao.LoanDAO;
import gov.riverbend.circ.dao.PatronDAO;
import gov.riverbend.circ.model.Fine;
import gov.riverbend.circ.model.Item;
import gov.riverbend.circ.model.Loan;
import gov.riverbend.circ.model.Patron;

import java.math.BigDecimal;
import java.sql.Connection;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.util.Date;
import java.util.Map;

/**
 * Checkout and check-in at the desk and at the kiosks. Every rule that can
 * refuse a transaction is checked here, inside the transaction.
 */
public class CirculationService {

    private final PatronDAO patrons = new PatronDAO();
    private final ItemDAO items = new ItemDAO();
    private final LoanDAO loans = new LoanDAO();
    private final FineDAO fines = new FineDAO();
    private final LoanPeriodCalculator periods;
    private final FineCalculator fineCalculator;
    private final HoldQueue holdQueue;
    private final CircConfig config;
    private final Map<String, Integer> borrowingLimits;

    public CirculationService(CircConfig config, LoanPeriodCalculator periods, FineCalculator fineCalculator,
                              HoldQueue holdQueue, Map<String, Integer> borrowingLimits) {
        this.config = config;
        this.periods = periods;
        this.fineCalculator = fineCalculator;
        this.holdQueue = holdQueue;
        this.borrowingLimits = borrowingLimits;
    }

    // @gloss implements .CheckoutHandler conf=0.85
    public Loan checkout(String cardNumber, String itemBarcode, long branchId) throws CircException, SQLException {
        Connection conn = ConnectionFactory.getConnection();
        try {
            conn.setAutoCommit(false);
            Patron patron = patrons.findByCard(conn, cardNumber);
            if (patron == null) {
                throw new CircException("NO_PATRON", "Card not found: " + cardNumber);
            }
            Item item = items.findByBarcode(conn, itemBarcode);
            if (item == null || item.isReference()) {
                throw new CircException("NOT_CIRCULATING", "Item does not circulate: " + itemBarcode);
            }
            if (items.isCheckedOut(conn, item.getId())) {
                throw new CircException("ALREADY_OUT", "Item is checked out; check it in first");
            }
            assertNotBlocked(patron, new Date());
            assertUnderLimit(conn, patron);

            Loan loan = new Loan();
            loan.setPatronId(patron.getId());
            loan.setItemId(item.getId());
            loan.setBranchId(branchId);
            loan.setCheckedOutAt(new Date());
            loan.setDueDate(periods.dueDate(item, loan.getCheckedOutAt()));
            loan.setId(loans.insert(conn, loan));
            conn.commit();
            return loan;
        } catch (SQLException e) {
            conn.rollback();
            throw e;
        } finally {
            ConnectionFactory.closeQuietly(conn);
        }
    }

    // @gloss implements .CheckinHandler conf=0.85
    public Loan checkin(String itemBarcode, long branchId) throws CircException, SQLException {
        Connection conn = ConnectionFactory.getConnection();
        try {
            conn.setAutoCommit(false);
            Item item = items.findByBarcode(conn, itemBarcode);
            Loan loan = item == null ? null : loans.findOpenByItem(conn, item.getId());
            if (loan == null) {
                throw new CircException("NOT_OUT", "Item is not checked out: " + itemBarcode);
            }
            Timestamp now = new Timestamp(System.currentTimeMillis());
            loans.markReturned(conn, loan.getId(), now);

            Patron patron = patrons.findById(conn, loan.getPatronId());
            BigDecimal owed = fineCalculator.overdueFine(loan, item, patron, now);
            if (owed.signum() > 0) {
                Fine fine = new Fine();
                fine.setPatronId(patron.getId());
                fine.setLoanId(loan.getId());
                fine.setFineType("OVERDUE");
                fine.setAmount(owed);
                fines.insert(conn, fine);
            }
            holdQueue.trapIfWanted(conn, item, branchId);
            conn.commit();
            return loan;
        } catch (SQLException e) {
            conn.rollback();
            throw e;
        } finally {
            ConnectionFactory.closeQuietly(conn);
        }
    }

    // @gloss implements .PatronBlockCheck scope=symbol conf=0.85
    void assertNotBlocked(Patron patron, Date today) throws CircException {
        if (patron.isExpired(today)) {
            throw new CircException("EXPIRED", "Card expired; renew at the desk");
        }
        if (patron.getBalanceOwed().compareTo(config.blockThreshold()) >= 0) {
            throw new CircException("BLOCKED", "Patron owes " + patron.getBalanceOwed() + "; borrowing is blocked");
        }
    }

    // @gloss implements .BorrowingLimitCheck conf=0.85
    void assertUnderLimit(Connection conn, Patron patron) throws CircException, SQLException {
        Integer limit = borrowingLimits.get(patron.getType().name());
        if (limit != null && patrons.countOpenLoans(conn, patron.getId()) >= limit.intValue()) {
            throw new CircException("LIMIT", "Borrowing limit of " + limit + " reached");
        }
    }
}
