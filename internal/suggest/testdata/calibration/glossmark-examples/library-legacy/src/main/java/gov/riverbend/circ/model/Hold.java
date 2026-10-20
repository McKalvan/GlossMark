/*
 * Riverbend Public Library - Circulation (CIRC)
 * Copyright (c) 2012 City of Riverbend. All rights reserved.
 */
package gov.riverbend.circ.model;

import java.util.Date;

/** A patron's request for the next available copy of a title. */
// @gloss represents lib:Hold conf=0.95
public class Hold {

    public static final String WAITING = "WAITING";
    public static final String TRAPPED = "TRAPPED";
    public static final String FILLED = "FILLED";
    public static final String EXPIRED = "EXPIRED";

    private long id;
    private long patronId;
    private long titleId;
    private long pickupBranchId;
    private Long trappedItemId;
    private Date placedAt;
    private Date trappedAt;
    private Date pickupBy;
    private String state = WAITING;

    public long getId() { return id; }
    public void setId(long id) { this.id = id; }

    public long getPatronId() { return patronId; }
    public void setPatronId(long patronId) { this.patronId = patronId; }

    public long getTitleId() { return titleId; }
    public void setTitleId(long titleId) { this.titleId = titleId; }

    public long getPickupBranchId() { return pickupBranchId; }
    public void setPickupBranchId(long pickupBranchId) { this.pickupBranchId = pickupBranchId; }

    public Long getTrappedItemId() { return trappedItemId; }
    public void setTrappedItemId(Long trappedItemId) { this.trappedItemId = trappedItemId; }

    public Date getPlacedAt() { return placedAt; }
    public void setPlacedAt(Date placedAt) { this.placedAt = placedAt; }

    public Date getTrappedAt() { return trappedAt; }
    public void setTrappedAt(Date trappedAt) { this.trappedAt = trappedAt; }

    public Date getPickupBy() { return pickupBy; }
    public void setPickupBy(Date pickupBy) { this.pickupBy = pickupBy; }

    public String getState() { return state; }
    public void setState(String state) { this.state = state; }
}
