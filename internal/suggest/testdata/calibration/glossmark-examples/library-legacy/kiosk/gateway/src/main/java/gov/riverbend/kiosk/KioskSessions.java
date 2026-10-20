/*
 * Riverbend Public Library - Kiosk gateway
 * Copyright (c) 2022 City of Riverbend. All rights reserved.
 */
package gov.riverbend.kiosk;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/** In-memory kiosk sessions; a session ends on EndSession or after the idle timeout in kiosk.json. */
public class KioskSessions {

    public static final class Session {
        private final String cardNumber;
        private final long branchId;
        private final long startedAt;

        Session(String cardNumber, long branchId) {
            this.cardNumber = cardNumber;
            this.branchId = branchId;
            this.startedAt = System.currentTimeMillis();
        }

        public String cardNumber() { return cardNumber; }
        public long branchId() { return branchId; }
        public long startedAt() { return startedAt; }
    }

    private final Map<String, Session> sessions = new ConcurrentHashMap<>();
    private final long idleMillis;

    public KioskSessions(long idleSeconds) {
        this.idleMillis = idleSeconds * 1000L;
    }

    public Session get(String id) {
        Session s = sessions.get(id);
        if (s == null || System.currentTimeMillis() - s.startedAt() > idleMillis) {
            sessions.remove(id);
            throw new IllegalStateException("session expired");
        }
        return s;
    }

    public String start(String sessionId, String cardNumber, long branchId) {
        sessions.put(sessionId, new Session(cardNumber, branchId));
        return sessionId;
    }
}
