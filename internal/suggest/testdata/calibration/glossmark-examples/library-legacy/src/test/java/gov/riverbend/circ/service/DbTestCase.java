/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.sql.Statement;

import org.junit.After;

/** Base class for tests that need the circ_test schema. Truncates after each test. */
public abstract class DbTestCase {

    protected Connection conn;

    protected void fixture(String name) throws IOException, SQLException {
        if (conn == null) {
            conn = DriverManager.getConnection(System.getProperty("circ.test.url", "jdbc:postgresql://localhost/circ_test"),
                "circ", System.getProperty("circ.test.password", "circ"));
        }
        String sql = new String(Files.readAllBytes(Paths.get("src/test/resources/fixtures", name)), "UTF-8");
        Statement st = conn.createStatement();
        try {
            st.execute(sql);
        } finally {
            st.close();
        }
    }

    protected RenewalService renewalService() throws IOException {
        CircConfig config = new CircConfig("src/test/resources/circulation.yaml");
        return new RenewalService(new LoanPeriodCalculator("config/loan-rules.json", config), config);
    }

    protected HoldQueue holdQueue() throws IOException {
        return new HoldQueue(new CircConfig("src/test/resources/circulation.yaml"));
    }

    @After
    public void truncate() throws SQLException {
        if (conn != null) {
            Statement st = conn.createStatement();
            try {
                st.execute("TRUNCATE loans, holds, fines RESTART IDENTITY CASCADE");
            } finally {
                st.close();
                conn.close();
                conn = null;
            }
        }
    }
}
