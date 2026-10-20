/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2019 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.service;

import gov.riverbend.circ.model.Item;
import gov.riverbend.circ.util.DateUtil;

import java.io.FileReader;
import java.io.IOException;
import java.util.Date;

import org.json.JSONObject;
import org.json.JSONTokener;

/**
 * Due dates. Loan periods per format live in config/loan-rules.json since
 * 2019 (CIRC-988); before that they were hard-coded here.
 */
// @gloss implements .LoanPeriodRule conf=0.85
public class LoanPeriodCalculator {

    private final JSONObject formats;
    private final CircConfig config;

    public LoanPeriodCalculator(String rulesPath, CircConfig config) throws IOException {
        FileReader r = new FileReader(rulesPath);
        try {
            this.formats = new JSONObject(new JSONTokener(r)).getJSONObject("formats");
        } finally {
            r.close();
        }
        this.config = config;
    }

    public int periodDays(Item item) {
        return formats.getJSONObject(item.getFormat().name()).getInt("loanDays");
    }

    public boolean isRenewable(Item item) {
        return formats.getJSONObject(item.getFormat().name()).optBoolean("renewable", true);
    }

    public Date dueDate(Item item, Date from) {
        Date due = DateUtil.addDays(from, periodDays(item));
        return DateUtil.nextOpenDay(due, config.closedDays());
    }
}
