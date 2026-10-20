package com.kestrelfreight.shipment.booking;

import com.kestrelfreight.shipment.model.Shipment;
import com.kestrelfreight.shipment.model.Shipment.ServiceLevel;
import jakarta.validation.Valid;
import jakarta.validation.constraints.DecimalMax;
import jakarta.validation.constraints.DecimalMin;
import jakarta.validation.constraints.FutureOrPresent;
import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotEmpty;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Pattern;
import jakarta.validation.constraints.Size;

import java.time.LocalDate;
import java.time.LocalTime;
import java.util.List;

public record BookShipmentRequest(
        @NotBlank @Size(max = 64) String shipperId,
        @NotNull @Valid AddressDto pickupAddress,
        @NotNull @Valid AddressDto deliveryAddress,
        @NotEmpty @Size(max = 20) List<@Valid @NotNull ParcelDto> parcels,
        @NotNull ServiceLevel serviceLevel,
        @NotNull @FutureOrPresent LocalDate requestedDate,
        LocalTime preferredWindowStart) {

    public record AddressDto(
            @NotBlank @Size(max = 120) String line1,
            @NotBlank @Size(max = 80) String city,
            @NotBlank @Size(max = 12) String postcode,
            @NotBlank @Pattern(regexp = "[A-Z]{2}") String country,
            @DecimalMin("-90.0") @DecimalMax("90.0") double lat,
            @DecimalMin("-180.0") @DecimalMax("180.0") double lng) {

        Shipment.Address toAddress() {
            return new Shipment.Address(line1, city, postcode, country, lat, lng);
        }
    }

    public record ParcelDto(
            @Min(1) @Max(31_500) int weightGrams,
            @Min(1) @Max(200) int lengthCm,
            @Min(1) @Max(120) int widthCm,
            @Min(1) @Max(120) int heightCm) {
    }
}
