package com.kestrelfreight.shipment.carrier;

import com.kestrelfreight.shipment.model.Carrier;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/carriers")
public class CarrierController {

    public record CarrierView(String code, String name, List<String> postcodePrefixes, int maxParcelWeightGrams,
                              boolean supportsExpress) {

        static CarrierView from(Carrier carrier) {
            return new CarrierView(carrier.getCode(), carrier.getName(), carrier.getPostcodePrefixes(),
                    carrier.getMaxParcelWeightGrams(), carrier.isSupportsExpress());
        }
    }

    private final CarrierRepository carriers;

    public CarrierController(CarrierRepository carriers) {
        this.carriers = carriers;
    }

    @GetMapping
    public List<CarrierView> list() {
        return carriers.findByActiveTrueOrderByPriorityAscCodeAsc().stream()
                .map(CarrierView::from)
                .toList();
    }
}
