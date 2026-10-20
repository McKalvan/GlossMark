/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2010 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.servlet;

import gov.riverbend.circ.service.CircConfig;
import gov.riverbend.circ.service.CirculationService;
import gov.riverbend.circ.service.FineCalculator;
import gov.riverbend.circ.service.HoldQueue;
import gov.riverbend.circ.service.LoanPeriodCalculator;
import gov.riverbend.circ.service.RenewalService;

import java.io.FileReader;
import java.io.IOException;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;

import javax.servlet.ServletContext;
import javax.servlet.ServletContextEvent;
import javax.servlet.ServletContextListener;

import org.json.JSONObject;
import org.json.JSONTokener;

/** Builds the services once and parks them in the ServletContext (no DI container here). */
public class CircContextListener implements ServletContextListener {

    public void contextInitialized(ServletContextEvent event) {
        ServletContext ctx = event.getServletContext();
        String dir = System.getenv("CIRC_CONFIG_DIR") != null ? System.getenv("CIRC_CONFIG_DIR") : "/etc/circ";
        try {
            CircConfig config = new CircConfig(dir + "/circulation.yaml");
            LoanPeriodCalculator periods = new LoanPeriodCalculator(dir + "/loan-rules.json", config);
            HoldQueue holdQueue = new HoldQueue(config);
            ctx.setAttribute("holdQueue", holdQueue);
            ctx.setAttribute("circulation", new CirculationService(config, periods, new FineCalculator(config),
                holdQueue, borrowingLimits(dir + "/patron-types.json")));
            ctx.setAttribute("renewals", new RenewalService(periods, config));
        } catch (IOException e) {
            throw new IllegalStateException("cannot read circulation config in " + dir, e);
        }
    }

    public void contextDestroyed(ServletContextEvent event) {
    }

    private Map<String, Integer> borrowingLimits(String path) throws IOException {
        Map<String, Integer> out = new HashMap<String, Integer>();
        FileReader r = new FileReader(path);
        try {
            JSONObject types = new JSONObject(new JSONTokener(r)).getJSONObject("patronTypes");
            Iterator<?> keys = types.keys();
            while (keys.hasNext()) {
                String k = (String) keys.next();
                out.put(k, Integer.valueOf(types.getJSONObject(k).getInt("maxItems")));
            }
        } finally {
            r.close();
        }
        return out;
    }
}
