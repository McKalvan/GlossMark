/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.dao;

import java.sql.Connection;
import java.sql.SQLException;

import javax.naming.InitialContext;
import javax.naming.NamingException;
import javax.sql.DataSource;

/** Looks up the container-managed pool (java:comp/env/jdbc/circ) once. */
public final class ConnectionFactory {

    private static DataSource dataSource;

    private ConnectionFactory() {
    }

    public static synchronized Connection getConnection() throws SQLException {
        if (dataSource == null) {
            try {
                InitialContext ctx = new InitialContext();
                dataSource = (DataSource) ctx.lookup("java:comp/env/jdbc/circ");
            } catch (NamingException e) {
                throw new SQLException("jdbc/circ not bound: " + e.getMessage());
            }
        }
        return dataSource.getConnection();
    }

    public static void closeQuietly(AutoCloseable c) {
        if (c == null) {
            return;
        }
        try {
            c.close();
        } catch (Exception ignored) {
            // nothing useful to do
        }
    }
}
