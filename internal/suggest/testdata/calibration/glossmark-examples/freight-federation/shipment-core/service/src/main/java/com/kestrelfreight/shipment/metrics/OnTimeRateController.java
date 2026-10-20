package com.kestrelfreight.shipment.metrics;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/metrics")
public class OnTimeRateController {

    private final OnTimeRateCalculator calculator;

    public OnTimeRateController(OnTimeRateCalculator calculator) {
        this.calculator = calculator;
    }

    @GetMapping("/on-time-rate")
    public OnTimeRateCalculator.OnTimeRate onTimeRate() {
        return calculator.current();
    }
}
