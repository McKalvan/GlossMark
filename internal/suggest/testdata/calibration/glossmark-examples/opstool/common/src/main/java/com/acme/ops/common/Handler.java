package com.acme.ops.common;

import java.util.Map;

// @gloss implements .LabelRenderer conf=0.9
public class Handler {

    private final String prefix;

    public Handler(String prefix) {
        this.prefix = prefix;
    }

    public String handle(Map<String, Object> data) {
        long serial = ((Number) data.get("serial")).longValue();
        String num = StringUtils.build(prefix, serial);
        StringBuilder z = new StringBuilder();
        z.append("^XA\n");
        z.append("^FO50,50^A0N,40,40^FD").append(data.get("shipTo")).append("^FS\n");
        z.append("^FO50,120^A0N,30,30^FDPO ").append(data.get("po")).append("^FS\n");
        z.append("^FO50,200^BCN,120,Y,N,N^FD(00)").append(num).append("^FS\n");
        z.append("^XZ\n");
        return z.toString();
    }

    public String doIt(Map<String, Object> data) {
        if (StringUtils.isBlank((String) data.get("shipTo"))) {
            throw new IllegalArgumentException("shipTo");
        }
        return handle(data);
    }
}
