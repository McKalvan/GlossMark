package com.kestrelfreight.shipment.label;

import com.kestrelfreight.shipment.model.Shipment;
import com.kestrelfreight.shipment.model.ShipmentRepository;
import com.kestrelfreight.shipment.model.ShippingLabel;
import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RestController;

import java.time.Clock;
import java.util.UUID;

@RestController
public class LabelController {

    static final MediaType ZPL = MediaType.parseMediaType("x-application/zpl");

    private final ShipmentRepository shipments;
    private final LabelRenderer renderer;
    private final Clock clock;

    public LabelController(ShipmentRepository shipments, LabelRenderer renderer, Clock clock) {
        this.shipments = shipments;
        this.renderer = renderer;
        this.clock = clock;
    }

    @GetMapping(value = "/shipments/{id}/label", produces = "x-application/zpl")
    @Transactional
    public ResponseEntity<String> label(@PathVariable UUID id) {
        return shipments.findWithParcelsById(id)
                .filter(shipment -> shipment.getStatus() != Shipment.Status.CANCELLED)
                .map(this::labelFor)
                .map(label -> ResponseEntity.ok()
                        .contentType(ZPL)
                        .header(HttpHeaders.CONTENT_DISPOSITION,
                                "inline; filename=\"" + label.getTrackingNumber() + ".zpl\"")
                        .body(label.getZpl()))
                .orElseGet(() -> ResponseEntity.notFound().build());
    }

    private ShippingLabel labelFor(Shipment shipment) {
        return shipment.getLabel().orElseGet(() -> {
            ShippingLabel label = new ShippingLabel(shipment, LabelSpec.FORMAT, renderer.render(shipment), clock.instant());
            shipment.attachLabel(label);
            return label;
        });
    }
}
