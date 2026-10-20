/*
 * Riverbend Public Library - Kiosk gateway
 * Copyright (c) 2022 City of Riverbend. All rights reserved.
 */
package gov.riverbend.kiosk;

import gov.riverbend.circ.model.Loan;
import gov.riverbend.circ.service.CircException;
import gov.riverbend.circ.service.CirculationService;
import gov.riverbend.circ.service.RenewalService;
import gov.riverbend.kiosk.api.v1.CheckoutRequest;
import gov.riverbend.kiosk.api.v1.CheckoutResponse;
import gov.riverbend.kiosk.api.v1.LoanSummary;
import gov.riverbend.kiosk.api.v1.RefusalCode;
import gov.riverbend.kiosk.api.v1.RenewRequest;
import gov.riverbend.kiosk.api.v1.RenewResponse;
import gov.riverbend.kiosk.api.v1.SelfCheckServiceGrpc;

import com.google.protobuf.Timestamp;

import io.grpc.stub.StreamObserver;

import java.sql.SQLException;
import java.util.Date;

/**
 * gRPC front for the kiosks. Reuses CirculationService so the desk and the
 * kiosks enforce the same rules; the kiosk only adds sessions and receipts.
 */
// @gloss implements .SelfCheckApi conf=0.55
public class SelfCheckGateway extends SelfCheckServiceGrpc.SelfCheckServiceImplBase {

    private final CirculationService circulation;
    private final RenewalService renewals;
    private final KioskSessions sessions;

    public SelfCheckGateway(CirculationService circulation, RenewalService renewals, KioskSessions sessions) {
        this.circulation = circulation;
        this.renewals = renewals;
        this.sessions = sessions;
    }

    @Override
    public void checkout(CheckoutRequest req, StreamObserver<CheckoutResponse> out) {
        KioskSessions.Session s = sessions.get(req.getSessionId());
        CheckoutResponse.Builder resp = CheckoutResponse.newBuilder();
        try {
            Loan loan = circulation.checkout(s.cardNumber(), req.getItemBarcode(), s.branchId());
            resp.setLoan(LoanSummary.newBuilder()
                .setItemBarcode(req.getItemBarcode())
                .setDueDate(ts(loan.getDueDate())));
        } catch (CircException e) {
            resp.setRefusal(refusal(e.getCode())).setMessage(e.getMessage());
        } catch (SQLException e) {
            resp.setRefusal(RefusalCode.REFUSAL_CODE_SEE_STAFF).setMessage("Please see staff at the desk");
        }
        out.onNext(resp.build());
        out.onCompleted();
    }

    @Override
    public void renew(RenewRequest req, StreamObserver<RenewResponse> out) {
        RenewResponse.Builder resp = RenewResponse.newBuilder();
        try {
            resp.setNewDueDate(ts(renewals.renew(req.getItemBarcode())));
        } catch (CircException e) {
            resp.setRefusal(refusal(e.getCode())).setMessage(e.getMessage());
        } catch (SQLException e) {
            resp.setRefusal(RefusalCode.REFUSAL_CODE_SEE_STAFF).setMessage("Please see staff at the desk");
        }
        out.onNext(resp.build());
        out.onCompleted();
    }

    private static RefusalCode refusal(String code) {
        if ("BLOCKED".equals(code) || "EXPIRED".equals(code)) {
            return RefusalCode.REFUSAL_CODE_BLOCKED;
        }
        if ("LIMIT".equals(code)) {
            return RefusalCode.REFUSAL_CODE_LIMIT;
        }
        if ("ON_HOLD".equals(code)) {
            return RefusalCode.REFUSAL_CODE_ON_HOLD;
        }
        if ("MAX_RENEWALS".equals(code)) {
            return RefusalCode.REFUSAL_CODE_MAX_RENEWALS;
        }
        if ("NOT_CIRCULATING".equals(code) || "ALREADY_OUT".equals(code)) {
            return RefusalCode.REFUSAL_CODE_NOT_CIRCULATING;
        }
        return RefusalCode.REFUSAL_CODE_SEE_STAFF;
    }

    private static Timestamp ts(Date d) {
        return Timestamp.newBuilder().setSeconds(d.getTime() / 1000).build();
    }
}
