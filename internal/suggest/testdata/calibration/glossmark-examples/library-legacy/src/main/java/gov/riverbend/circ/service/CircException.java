/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

/** A circulation rule refused the transaction; the message is shown to desk staff. */
public class CircException extends Exception {

    private static final long serialVersionUID = 1L;

    private final String code;

    public CircException(String code, String message) {
        super(message);
        this.code = code;
    }

    public String getCode() {
        return code;
    }
}
