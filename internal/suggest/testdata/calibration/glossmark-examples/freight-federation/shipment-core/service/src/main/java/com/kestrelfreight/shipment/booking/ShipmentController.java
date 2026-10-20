package com.kestrelfreight.shipment.booking;

import jakarta.validation.Valid;
import org.springframework.http.HttpStatus;
import org.springframework.http.ProblemDetail;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.servlet.support.ServletUriComponentsBuilder;

import java.net.URI;
import java.util.UUID;

@RestController
@RequestMapping("/shipments")
public class ShipmentController {

    private final BookingService bookingService;

    public ShipmentController(BookingService bookingService) {
        this.bookingService = bookingService;
    }

    @PostMapping
    public ResponseEntity<ShipmentView> book(@Valid @RequestBody BookShipmentRequest request) {
        ShipmentView booked = bookingService.book(request);
        URI location = ServletUriComponentsBuilder.fromCurrentRequest()
                .path("/{id}")
                .buildAndExpand(booked.id())
                .toUri();
        return ResponseEntity.created(location).body(booked);
    }

    @GetMapping("/{id}")
    public ResponseEntity<ShipmentView> get(@PathVariable UUID id) {
        return ResponseEntity.of(bookingService.find(id));
    }

    @ExceptionHandler(BookingService.NoCarrierAvailableException.class)
    ProblemDetail noCarrier(BookingService.NoCarrierAvailableException e) {
        ProblemDetail problem = ProblemDetail.forStatusAndDetail(HttpStatus.UNPROCESSABLE_ENTITY, e.getMessage());
        problem.setTitle("No carrier available");
        return problem;
    }

    @ExceptionHandler(IllegalArgumentException.class)
    ProblemDetail invalidBooking(IllegalArgumentException e) {
        ProblemDetail problem = ProblemDetail.forStatusAndDetail(HttpStatus.BAD_REQUEST, e.getMessage());
        problem.setTitle("Invalid booking");
        return problem;
    }
}
