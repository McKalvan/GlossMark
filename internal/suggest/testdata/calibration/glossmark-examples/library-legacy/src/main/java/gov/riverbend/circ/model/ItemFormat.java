/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2011 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

/** Mirrors ItemFormat in config/enums.yaml; EnumRegistry fails startup if they differ. */
// @gloss represents lib:ItemFormat conf=0.95
public enum ItemFormat {
    BOOK,
    LARGE_PRINT,
    DVD,
    AUDIOBOOK,
    MAGAZINE;

    public static ItemFormat fromCode(String code) {
        return valueOf(code.trim().toUpperCase());
    }
}
