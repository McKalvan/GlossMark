/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.report;

import gov.riverbend.circ.dao.BorrowerDAO;
import gov.riverbend.circ.dao.ConnectionFactory;
import gov.riverbend.circ.model.Borrower;

import java.io.PrintWriter;
import java.sql.Connection;
import java.sql.SQLException;
import java.util.List;

/**
 * The CSV the state library asks for every January ("active borrowers by
 * class"). The state's template still says "borrower", so this report was
 * never moved to Patron.
 */
@SuppressWarnings("deprecation")
public class BorrowerActivityReport {

    public void write(java.sql.Date since, PrintWriter out) throws SQLException {
        Connection conn = ConnectionFactory.getConnection();
        try {
            List<Borrower> rows = new BorrowerDAO().findActiveSince(conn, since);
            out.println("borrower_id,barcode,name,class,items_out");
            for (Borrower b : rows) {
                out.printf("%d,%s,\"%s\",%s,%d%n", b.getBorrowerId(), b.getBarcode(),
                    b.getName().replace("\"", "\"\""), b.getBorrowerClass(), b.getItemsOut());
            }
        } finally {
            ConnectionFactory.closeQuietly(conn);
        }
    }
}
