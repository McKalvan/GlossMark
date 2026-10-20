package com.acme.ops.common;

public class Handler {
    // @gloss implements wh:Replenishment
    /** Top up the zone's pick faces from reserve. */
    public void doIt(String zone) {
        System.out.println(zone);
    }

    public void handle(String s) {
        System.out.println(s);
    }
}
